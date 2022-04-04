//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package upstream

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
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
	generalWriteTimeout = time.Second * 1
	tlsHandshakeTimeout = time.Second * 5
)

var (
	nopLogger = zap.NewNop()
)

// Upstream represents a DNS upstream.
type Upstream interface {
	// ExchangeContext exchanges query message m to the upstream, and returns
	// response. It MUST NOT keep or modify m.
	ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error)

	// CloseIdleConnections closes any connections in the Upstream which
	// now sitting idle. It does not interrupt any connections currently in use.
	CloseIdleConnections()
}

type Opt struct {
	// DialAddr specifies the address the upstream will
	// actually dial to.
	// Not implemented for doh upstreams with http/3.
	DialAddr string

	// Socks5 specifies the socks5 proxy server that the upstream
	// will connect though.
	// Not implemented for udp upstreams and doh upstreams with http/3.
	Socks5 string

	// SoMark specifies the mark for each packet sent through this upstream.
	// Not implemented for doh upstreams with http/3
	SoMark int

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
	// Default is 1.
	MaxConns int

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

	switch addrURL.Scheme {
	case "", "udp":
		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 53)
		ut := &Transport{
			Logger: opt.Logger,
			DialFunc: func(ctx context.Context) (net.Conn, error) {
				d := net.Dialer{
					Control: getSetMarkFunc(opt.SoMark),
				}
				return d.DialContext(ctx, "udp", dialAddr)
			},
			WriteFunc: dnsutils.WriteMsgToUDP,
			ReadFunc: func(c io.Reader) (*dns.Msg, int, error) {
				return dnsutils.ReadMsgFromUDP(c, 4096)
			},
			EnablePipeline: true,
			MaxConns:       opt.MaxConns,
			IdleTimeout:    time.Second * 60,
		}
		tt := &Transport{
			Logger: opt.Logger,
			DialFunc: func(ctx context.Context) (net.Conn, error) {
				d := net.Dialer{
					Control: getSetMarkFunc(opt.SoMark),
				}
				return d.DialContext(ctx, "tcp", dialAddr)
			},
			WriteFunc: dnsutils.WriteMsgToTCP,
			ReadFunc:  dnsutils.ReadMsgFromTCP,
		}
		return &udpWithFallback{
			u: ut,
			t: tt,
		}, nil
	case "tcp":
		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 53)
		t := &Transport{
			Logger: opt.Logger,
			DialFunc: func(ctx context.Context) (net.Conn, error) {
				return dialTCP(ctx, dialAddr, opt.Socks5, opt.SoMark)
			},
			WriteFunc:      dnsutils.WriteMsgToTCP,
			ReadFunc:       dnsutils.ReadMsgFromTCP,
			IdleTimeout:    opt.IdleTimeout,
			EnablePipeline: opt.EnablePipeline,
			MaxConns:       opt.MaxConns,
		}
		return t, nil
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
		t := &Transport{
			Logger: opt.Logger,
			DialFunc: func(ctx context.Context) (net.Conn, error) {
				conn, err := dialTCP(ctx, dialAddr, opt.Socks5, opt.SoMark)
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
		return t, nil
	case "https":
		idleConnTimeout := time.Second * 30
		if opt.IdleTimeout > 0 {
			idleConnTimeout = opt.IdleTimeout
		}
		maxConn := 1
		if opt.MaxConns > 0 {
			maxConn = opt.MaxConns
		}

		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 443)
		var t http.RoundTripper
		if opt.EnableHTTP3 {
			t = &h3rt{
				logger:    opt.Logger,
				tlsConfig: opt.TLSConfig,
				quicConfig: &quic.Config{
					TokenStore:                     quic.NewLRUTokenStore(4, 8),
					InitialStreamReceiveWindow:     64 * 1024,
					MaxStreamReceiveWindow:         128 * 1024,
					InitialConnectionReceiveWindow: 256 * 1024,
					MaxConnectionReceiveWindow:     512 * 1024,
				},
			}
		} else {
			t1 := &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
					return dialTCP(ctx, dialAddr, opt.Socks5, opt.SoMark)
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

		return &DoHUpstream{
			EndPoint: addr,
			Client:   &http.Client{Transport: t},
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
	u *Transport
	t *Transport
}

func (u *udpWithFallback) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	m, err := u.u.ExchangeContext(ctx, q)
	if err != nil {
		return nil, err
	}
	if m.Truncated {
		u.u.logger().Warn("truncated udp msg received, retrying tcp")
		return u.t.ExchangeContext(ctx, q)
	}
	return m, nil
}

func (u *udpWithFallback) CloseIdleConnections() {
	u.u.CloseIdleConnections()
	u.t.CloseIdleConnections()
}
