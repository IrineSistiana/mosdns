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
	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/bootstrap"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/doh"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/transport"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
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

	// EventObserver can observe connection events.
	// Note: Not Implemented for HTTP/3 upstreams.
	EventObserver EventObserver
}

func NewUpstream(addr string, opt Opt) (Upstream, error) {
	if opt.Logger == nil {
		opt.Logger = mlog.Nop()
	}
	if opt.EventObserver == nil {
		opt.EventObserver = nopEO{}
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
		Resolver: bootstrap.NewBootstrap(opt.Bootstrap),
		Control: getSocketControlFunc(socketOpts{
			so_mark:        opt.SoMark,
			bind_to_device: opt.BindToDevice,
		}),
	}

	switch addrURL.Scheme {
	case "", "udp":
		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 53)
		uto := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				c, err := dialer.DialContext(ctx, "udp", dialAddr)
				c = wrapConn(c, opt.EventObserver)
				return c, err
			},
			WriteFunc: dnsutils.WriteMsgToUDP,
			ReadFunc: func(c io.Reader) (*dns.Msg, int, error) {
				return dnsutils.ReadMsgFromUDP(c, 4096)
			},
			IdleTimeout: time.Minute * 5,
		}
		tto := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				c, err := dialer.DialContext(ctx, "tcp", dialAddr)
				c = wrapConn(c, opt.EventObserver)
				return c, err
			},
			WriteFunc: dnsutils.WriteMsgToTCP,
			ReadFunc:  dnsutils.ReadMsgFromTCP,
		}
		return &udpWithFallback{
			u: transport.NewPipelineTransport(transport.PipelineOpts{IOOpts: uto, MaxConn: 1}),
			t: transport.NewReuseConnTransport(transport.ReuseConnOpts{IOOpts: tto}),
		}, nil
	case "tcp":
		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 53)
		to := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				c, err := dialTCP(ctx, dialAddr, opt.Socks5, dialer)
				c = wrapConn(c, opt.EventObserver)
				return c, err
			},
			WriteFunc:   dnsutils.WriteMsgToTCP,
			ReadFunc:    dnsutils.ReadMsgFromTCP,
			IdleTimeout: opt.IdleTimeout,
		}
		if opt.EnablePipeline {
			return transport.NewPipelineTransport(transport.PipelineOpts{IOOpts: to, MaxConn: opt.MaxConns}), nil
		}
		return transport.NewReuseConnTransport(transport.ReuseConnOpts{IOOpts: to}), nil
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
		to := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				conn, err := dialTCP(ctx, dialAddr, opt.Socks5, dialer)
				if err != nil {
					return nil, err
				}
				conn = wrapConn(conn, opt.EventObserver)
				tlsConn := tls.Client(conn, tlsConfig)
				if err := tlsConn.HandshakeContext(ctx); err != nil {
					tlsConn.Close()
					return nil, err
				}

				return tlsConn, nil
			},
			WriteFunc:   dnsutils.WriteMsgToTCP,
			ReadFunc:    dnsutils.ReadMsgFromTCP,
			IdleTimeout: opt.IdleTimeout,
		}
		if opt.EnablePipeline {
			return transport.NewPipelineTransport(transport.PipelineOpts{IOOpts: to, MaxConn: opt.MaxConns}), nil
		}
		return transport.NewReuseConnTransport(transport.ReuseConnOpts{IOOpts: to}), nil
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
			t = &http3.RoundTripper{
				TLSClientConfig: opt.TLSConfig,
				QuicConfig: &quic.Config{
					TokenStore:                     quic.NewLRUTokenStore(4, 8),
					InitialStreamReceiveWindow:     4 * 1024,
					MaxStreamReceiveWindow:         4 * 1024,
					InitialConnectionReceiveWindow: 8 * 1024,
					MaxConnectionReceiveWindow:     64 * 1024,
					MaxIncomingStreams:             100,
					MaxIncomingUniStreams:          100,
				},
				Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
					ua, err := net.ResolveUDPAddr("udp", dialAddr) // TODO: Support bootstrap.
					if err != nil {
						return nil, err
					}
					return quic.DialEarly(ctx, conn, ua, tlsCfg, cfg)
				},
			}
		} else {
			t1 := &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
					c, err := dialTCP(ctx, dialAddr, opt.Socks5, dialer)
					c = wrapConn(c, opt.EventObserver)
					return c, err
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
	u *transport.PipelineTransport
	t *transport.ReuseConnTransport
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
