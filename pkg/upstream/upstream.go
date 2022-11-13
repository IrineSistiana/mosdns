/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package upstream

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v4/pkg/upstream/bootstrap"
	"github.com/IrineSistiana/mosdns/v4/pkg/upstream/doh"
	"github.com/IrineSistiana/mosdns/v4/pkg/upstream/h3roundtripper"
	"github.com/IrineSistiana/mosdns/v4/pkg/upstream/transport"
	"github.com/lucas-clemente/quic-go"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	tlsHandshakeTimeout = time.Second * 5
)

// Upstream represents a DNS upstream.
type Upstream interface {
	// ExchangeContext exchanges query message m to the upstream, and returns
	// response. It MUST NOT keep or modify m.
	ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error)

	io.Closer
}

type Opt struct {
	// DialAddr specifies the address the upstream will
	// actually dial to.
	DialAddr string

	// Socks5 specifies the socks5 proxy server that the upstream
	// will connect though.
	// Not implemented for udp upstreams and doh upstreams with http/3.
	Socks5 string

	// SoMark sets the socket SO_MARK option in unix system.
	SoMark int

	// BindToDevice sets the socket SO_BINDTODEVICE option in unix system.
	BindToDevice string

	// IdleTimeout specifies the idle timeout for long-connections.
	// Available for TCP, DoT, DoH.
	// If negative, TCP, DoT will not reuse connections.
	// Default: TCP, DoT: 10s , DoH: 30s.
	IdleTimeout time.Duration

	// EnablePipeline enables query pipelining support as RFC 7766 6.2.1.1 suggested.
	// Available for TCP, DoT upstream with IdleTimeout >= 0.
	EnablePipeline bool

	// EnableHTTP3 enables HTTP/3 protocol for DoH upstream.
	EnableHTTP3 bool

	// MaxConns limits the total number of connections, including connections
	// in the dialing states.
	// Implemented for TCP/DoT pipeline enabled upstreams and DoH upstreams.
	// Default is 2.
	MaxConns int

	// Bootstrap specifies a plain dns server for the go runtime to solve the
	// domain of the upstream server. It SHOULD be an IP address. Custom port
	// is supported.
	// Note: Use a domain address may cause dead resolve loop and additional
	// latency to dial upstream server.
	// HTTP3 is not supported.
	Bootstrap string

	// TLSConfig specifies the tls.Config that the TLS client will use.
	// Available for DoT, DoH upstreams.
	TLSConfig *tls.Config

	// Logger specifies the logger that the upstream will use.
	Logger *zap.Logger
}

func NewUpstream(addr string, opt *Opt) (Upstream, error) {
	if opt == nil {
		opt = new(Opt)
	}

	// parse protocol and server addr
	if !strings.Contains(addr, "://") {
		addr = "udp://" + addr
	}
	addrURL, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid server address, %w", err)
	}

	dialer := &net.Dialer{
		Resolver: bootstrap.NewPlainBootstrap(opt.Bootstrap),
		Control: getSocketControlFunc(socketOpts{
			so_mark:        opt.SoMark,
			bind_to_device: opt.BindToDevice,
		}),
	}

	switch addrURL.Scheme {
	case "", "udp":
		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 53)

		uto := transport.Opts{
			Logger: opt.Logger,
			DialFunc: func(ctx context.Context) (net.Conn, error) {
				return dialer.DialContext(ctx, "udp", dialAddr)
			},
			WriteFunc: dnsutils.WriteMsgToUDP,
			ReadFunc: func(c io.Reader) (*dns.Msg, int, error) {
				return dnsutils.ReadMsgFromUDP(c, 4096)
			},
			EnablePipeline: true,
			MaxConns:       opt.MaxConns,
			IdleTimeout:    time.Second * 60,
		}
		ut, err := transport.NewTransport(uto)
		if err != nil {
			return nil, fmt.Errorf("cannot init udp transport, %w", err)
		}
		tto := transport.Opts{
			Logger: opt.Logger,
			DialFunc: func(ctx context.Context) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", dialAddr)
			},
			WriteFunc: dnsutils.WriteMsgToTCP,
			ReadFunc:  dnsutils.ReadMsgFromTCP,
		}
		tt, err := transport.NewTransport(tto)
		if err != nil {
			return nil, fmt.Errorf("cannot init tcp transport, %w", err)
		}
		return &udpWithFallback{
			u: ut,
			t: tt,
		}, nil
	case "tcp":
		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 53)
		to := transport.Opts{
			Logger: opt.Logger,
			DialFunc: func(ctx context.Context) (net.Conn, error) {
				return dialTCP(ctx, dialAddr, opt.Socks5, dialer)
			},
			WriteFunc:      dnsutils.WriteMsgToTCP,
			ReadFunc:       dnsutils.ReadMsgFromTCP,
			IdleTimeout:    opt.IdleTimeout,
			EnablePipeline: opt.EnablePipeline,
			MaxConns:       opt.MaxConns,
		}
		return transport.NewTransport(to)
	case "tls":
		var tlsConfig *tls.Config
		if opt.TLSConfig != nil {
			tlsConfig = opt.TLSConfig.Clone()
		} else {
			tlsConfig = new(tls.Config)
		}
		if len(tlsConfig.ServerName) == 0 {
			tlsConfig.ServerName = tryRemovePort(addrURL.Host)
		}

		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 853)
		to := transport.Opts{
			Logger: opt.Logger,
			DialFunc: func(ctx context.Context) (net.Conn, error) {
				conn, err := dialTCP(ctx, dialAddr, opt.Socks5, dialer)
				if err != nil {
					return nil, err
				}
				tlsConn := tls.Client(conn, tlsConfig)
				if err := tlsConn.HandshakeContext(ctx); err != nil {
					tlsConn.Close()
					return nil, err
				}
				return tlsConn, nil
			},
			WriteFunc:      dnsutils.WriteMsgToTCP,
			ReadFunc:       dnsutils.ReadMsgFromTCP,
			IdleTimeout:    opt.IdleTimeout,
			EnablePipeline: opt.EnablePipeline,
			MaxConns:       opt.MaxConns,
		}
		return transport.NewTransport(to)
	case "https":
		idleConnTimeout := time.Second * 30
		if opt.IdleTimeout > 0 {
			idleConnTimeout = opt.IdleTimeout
		}
		maxConn := 2
		if opt.MaxConns > 0 {
			maxConn = opt.MaxConns
		}

		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 443)
		var t http.RoundTripper
		var addonCloser io.Closer // udpConn
		if opt.EnableHTTP3 {
			lc := net.ListenConfig{Control: getSocketControlFunc(socketOpts{so_mark: opt.SoMark, bind_to_device: opt.BindToDevice})}
			conn, err := lc.ListenPacket(context.Background(), "udp", "")
			if err != nil {
				return nil, fmt.Errorf("failed to init udp socket for quic")
			}
			addonCloser = conn
			t = &h3roundtripper.H3RTHelper{
				Logger:    opt.Logger,
				TLSConfig: opt.TLSConfig,
				QUICConfig: &quic.Config{
					TokenStore:                     quic.NewLRUTokenStore(4, 8),
					InitialStreamReceiveWindow:     4 * 1024,
					MaxStreamReceiveWindow:         4 * 1024,
					InitialConnectionReceiveWindow: 8 * 1024,
					MaxConnectionReceiveWindow:     64 * 1024,
				},
				DialFunc: func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
					ua, err := net.ResolveUDPAddr("udp", dialAddr) // TODO: Support bootstrap.
					if err != nil {
						return nil, err
					}
					return quic.DialEarlyContext(ctx, conn, ua, addrURL.Host, tlsCfg, cfg)
				},
			}
		} else {
			t1 := &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
					return dialTCP(ctx, dialAddr, opt.Socks5, dialer)
				},
				TLSClientConfig:     opt.TLSConfig,
				TLSHandshakeTimeout: tlsHandshakeTimeout,
				IdleConnTimeout:     idleConnTimeout,

				// MaxConnsPerHost and MaxIdleConnsPerHost should be equal.
				// Otherwise, it might seriously affect the efficiency of connection reuse.
				MaxConnsPerHost:     maxConn,
				MaxIdleConnsPerHost: maxConn,
			}

			t2, err := http2.ConfigureTransports(t1)
			if err != nil {
				return nil, fmt.Errorf("failed to upgrade http2 support, %w", err)
			}
			t2.ReadIdleTimeout = time.Second * 30
			t2.PingTimeout = time.Second * 5
			t = t1
		}

		return &doh.Upstream{
			EndPoint:    addr,
			Client:      &http.Client{Transport: t},
			AddOnCloser: addonCloser,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol [%s]", addrURL.Scheme)
	}
}

func getDialAddrWithPort(host, dialAddr string, defaultPort int) string {
	addr := host
	if len(dialAddr) > 0 {
		addr = dialAddr
	}
	_, _, err := net.SplitHostPort(addr)
	if err != nil { // no port, add it.
		return net.JoinHostPort(strings.Trim(addr, "[]"), strconv.Itoa(defaultPort))
	}
	return addr
}

func tryRemovePort(s string) string {
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return s
	}
	return host
}

type udpWithFallback struct {
	u *transport.Transport
	t *transport.Transport
}

func (u *udpWithFallback) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	m, err := u.u.ExchangeContext(ctx, q)
	if err != nil {
		return nil, err
	}
	if m.Truncated {
		return u.t.ExchangeContext(ctx, q)
	}
	return m, nil
}

func (u *udpWithFallback) Close() error {
	u.u.Close()
	u.t.Close()
	return nil
}
