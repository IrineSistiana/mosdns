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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/bootstrap"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/doh"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/doq"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/transport"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

const (
	tlsHandshakeTimeout = time.Second * 3
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
	// actually dial to in the network layer by overwriting
	// the address inferred from upstream url.
	// It won't affect high level layers. (e.g. SNI, HTTP HOST header won't be changed).
	// Can be an IP or a domain. Port is optional.
	// Tips: If the upstream url host is a domain, specific an IP address
	// here can skip resolving ip of this domain.
	DialAddr string

	// Socks5 specifies the socks5 proxy server that the upstream
	// will connect though.
	// Not implemented for udp based protocols (aka. dns over udp, http3, quic).
	Socks5 string

	// SoMark sets the socket SO_MARK option in unix system.
	SoMark int

	// BindToDevice sets the socket SO_BINDTODEVICE option in unix system.
	BindToDevice string

	// IdleTimeout specifies the idle timeout for long-connections.
	// Available for TCP, DoT, DoH.
	// Default: TCP, DoT: 10s , DoH, DoQ: 30s.
	IdleTimeout time.Duration

	// EnablePipeline enables query pipelining support as RFC 7766 6.2.1.1 suggested.
	// Available for TCP, DoT upstream with IdleTimeout >= 0.
	// Note: There is no fallback.
	EnablePipeline bool

	// EnableHTTP3 enables HTTP/3 protocol for DoH upstream.
	// Note: There is no fallback.
	EnableHTTP3 bool

	// MaxConns limits the total number of connections, including connections
	// in the dialing states.
	// Implemented for TCP/DoT pipeline enabled upstream and DoH upstream.
	// Default is 2.
	MaxConns int

	// Bootstrap specifies a plain dns server to solve the
	// upstream server domain address.
	// It must be an IP address. Port is optional.
	Bootstrap string

	// Bootstrap version. One of 0 (default equals 4), 4, 6.
	// TODO: Support dual-stack.
	BootstrapVer int

	// TLSConfig specifies the tls.Config that the TLS client will use.
	// Available for DoT, DoH upstream.
	TLSConfig *tls.Config

	// Logger specifies the logger that the upstream will use.
	Logger *zap.Logger

	// EventObserver can observe connection events.
	// Not implemented for udp based protocols (dns over udp, http3, quic).
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

	// If host is a ipv6 without port, it will be in []. This will cause err when
	// split and join address and port. Try to remove brackets now.
	addrUrlHost := tryTrimIpv6Brackets(addrURL.Host)

	dialer := &net.Dialer{
		Control: getSocketControlFunc(socketOpts{
			so_mark:        opt.SoMark,
			bind_to_device: opt.BindToDevice,
		}),
	}

	var bootstrapAp netip.AddrPort
	if s := opt.Bootstrap; len(s) > 0 {
		bootstrapAp, err = parseBootstrapAp(s)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap, %w", err)
		}
	}

	newUdpAddrResolveFunc := func(defaultPort uint16) (func(ctx context.Context) (*net.UDPAddr, error), error) {
		host, port, err := parseDialAddr(addrUrlHost, opt.DialAddr, defaultPort)
		if err != nil {
			return nil, err
		}

		if addr, err := netip.ParseAddr(host); err == nil { // host is an ip.
			ua := net.UDPAddrFromAddrPort(netip.AddrPortFrom(addr, port))
			return func(ctx context.Context) (*net.UDPAddr, error) {
				return ua, nil
			}, nil
		} else { // Not an ip, assuming it's a domain name.
			if bootstrapAp.IsValid() {
				// Bootstrap enabled.
				bs, err := bootstrap.New(host, port, bootstrapAp, opt.BootstrapVer, opt.Logger)
				if err != nil {
					return nil, err
				}

				return func(ctx context.Context) (*net.UDPAddr, error) {
					s, err := bs.GetAddrPortStr(ctx)
					if err != nil {
						return nil, fmt.Errorf("bootstrap failed, %w", err)
					}
					return net.ResolveUDPAddr("udp", s)
				}, nil
			} else {
				// Bootstrap disabled.
				dialAddr := joinPort(host, port)
				return func(ctx context.Context) (*net.UDPAddr, error) {
					return net.ResolveUDPAddr("udp", dialAddr)
				}, nil
			}
		}
	}

	newTcpDialer := func(dialAddrMustBeIp bool, defaultPort uint16) (func(ctx context.Context) (net.Conn, error), error) {
		host, port, err := parseDialAddr(addrUrlHost, opt.DialAddr, defaultPort)
		if err != nil {
			return nil, err
		}

		// Socks5 enabled.
		if s5Addr := opt.Socks5; len(s5Addr) > 0 {
			socks5Dialer, err := proxy.SOCKS5("tcp", s5Addr, nil, dialer)
			if err != nil {
				return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
			}

			contextDialer := socks5Dialer.(proxy.ContextDialer)
			dialAddr := net.JoinHostPort(host, strconv.Itoa(int(port)))
			return func(ctx context.Context) (net.Conn, error) {
				return contextDialer.DialContext(ctx, "tcp", dialAddr)
			}, nil
		}

		if _, err := netip.ParseAddr(host); err == nil {
			// Host is an ip addr. No need to resolve it.
			dialAddr := net.JoinHostPort(host, strconv.Itoa(int(port)))
			return func(ctx context.Context) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", dialAddr)
			}, nil
		} else {
			if dialAddrMustBeIp {
				return nil, errors.New("addr must be an ip address")
			}
			// Host is not an ip addr, assuming it is a domain.
			if bootstrapAp.IsValid() {
				// Bootstrap enabled.
				bs, err := bootstrap.New(host, port, bootstrapAp, opt.BootstrapVer, opt.Logger)
				if err != nil {
					return nil, err
				}

				return func(ctx context.Context) (net.Conn, error) {
					dialAddr, err := bs.GetAddrPortStr(ctx)
					if err != nil {
						return nil, fmt.Errorf("bootstrap failed, %w", err)
					}
					return dialer.DialContext(ctx, "tcp", dialAddr)
				}, nil
			} else {
				// Bootstrap disabled.
				dialAddr := net.JoinHostPort(host, strconv.Itoa(int(port)))
				return func(ctx context.Context) (net.Conn, error) {
					return dialer.DialContext(ctx, "tcp", dialAddr)
				}, nil
			}
		}
	}

	switch addrURL.Scheme {
	case "", "udp":
		const defaultPort = 53
		host, port, err := parseDialAddr(addrUrlHost, opt.DialAddr, defaultPort)
		if err != nil {
			return nil, err
		}
		if _, err := netip.ParseAddr(host); err != nil {
			return nil, fmt.Errorf("addr must be an ip address, %w", err)
		}
		dialAddr := joinPort(host, port)
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
		const defaultPort = 53
		tcpDialer, err := newTcpDialer(true, defaultPort)
		if err != nil {
			return nil, fmt.Errorf("failed to init tcp dialer, %w", err)
		}
		to := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				c, err := tcpDialer(ctx)
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
		const defaultPort = 853
		tlsConfig := opt.TLSConfig.Clone()
		if tlsConfig == nil {
			tlsConfig = new(tls.Config)
		}
		if len(tlsConfig.ServerName) == 0 {
			tlsConfig.ServerName = tryRemovePort(addrUrlHost)
		}

		tcpDialer, err := newTcpDialer(false, defaultPort)
		if err != nil {
			return nil, fmt.Errorf("failed to init tcp dialer, %w", err)
		}
		to := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				conn, err := tcpDialer(ctx)
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
		const defaultPort = 443
		idleConnTimeout := time.Second * 30
		if opt.IdleTimeout > 0 {
			idleConnTimeout = opt.IdleTimeout
		}
		maxConn := 2
		if opt.MaxConns > 0 {
			maxConn = opt.MaxConns
		}

		var t http.RoundTripper
		var addonCloser io.Closer
		if opt.EnableHTTP3 {
			udpBootstrap, err := newUdpAddrResolveFunc(defaultPort)
			if err != nil {
				return nil, fmt.Errorf("failed to init udp addr bootstrap, %w", err)
			}

			lc := net.ListenConfig{Control: getSocketControlFunc(socketOpts{so_mark: opt.SoMark, bind_to_device: opt.BindToDevice})}
			conn, err := lc.ListenPacket(context.Background(), "udp", "")
			if err != nil {
				return nil, fmt.Errorf("failed to init udp socket for quic")
			}
			quicTransport := &quic.Transport{
				Conn: conn,
			}
			addonCloser = quicTransport
			t = &http3.RoundTripper{
				TLSClientConfig: opt.TLSConfig,
				QuicConfig:      newDefaultQuicConfig(),
				Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
					ua, err := udpBootstrap(ctx)
					if err != nil {
						return nil, err
					}
					return quicTransport.DialEarly(ctx, ua, tlsCfg, cfg)
				},
			}
		} else {
			tcpDialer, err := newTcpDialer(false, defaultPort)
			if err != nil {
				return nil, fmt.Errorf("failed to init tcp dialer, %w", err)
			}
			t1 := &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
					c, err := tcpDialer(ctx)
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

		return &dohWithClose{
			u:      doh.NewUpstream(addr, &http.Client{Transport: t}),
			closer: addonCloser,
		}, nil
	case "quic", "doq":
		const defaultPort = 853
		tlsConfig := opt.TLSConfig.Clone()
		if tlsConfig == nil {
			tlsConfig = new(tls.Config)
		}
		if len(tlsConfig.ServerName) == 0 {
			tlsConfig.ServerName = tryRemovePort(addrUrlHost)
		}
		tlsConfig.NextProtos = doq.DoqAlpn

		quicConfig := newDefaultQuicConfig()
		if opt.IdleTimeout > 0 {
			quicConfig.MaxIdleTimeout = opt.IdleTimeout
		}

		udpBootstrap, err := newUdpAddrResolveFunc(defaultPort)
		if err != nil {
			return nil, fmt.Errorf("failed to init udp addr bootstrap, %w", err)
		}

		srk, _, err := utils.InitQUICSrkFromIfaceMac()
		if err != nil {
			opt.Logger.Warn("failed to init quic stateless reset key, it will be disabled", zap.Error(err))
		}

		lc := net.ListenConfig{Control: getSocketControlFunc(socketOpts{so_mark: opt.SoMark, bind_to_device: opt.BindToDevice})}
		uc, err := lc.ListenPacket(context.Background(), "udp", "")
		if err != nil {
			return nil, fmt.Errorf("failed to init udp socket for quic")
		}

		t := &quic.Transport{
			Conn:              uc,
			StatelessResetKey: (*quic.StatelessResetKey)(srk),
		}

		dialer := func(ctx context.Context) (quic.Connection, error) {
			ua, err := udpBootstrap(ctx)
			if err != nil {
				return nil, fmt.Errorf("bootstrap failed, %w", err)
			}
			var c quic.Connection
			ec, err := t.DialEarly(ctx, ua, tlsConfig, quicConfig)
			if err != nil {
				return nil, err
			}

			// This is a workaround to
			// 1. recover from strange 0rtt rejected err.
			// 2. avoid NextConnection might block forever.
			// TODO: Remove this workaround.
			select {
			case <-ctx.Done():
				err := context.Cause(ctx)
				ec.CloseWithError(0, "")
				return nil, err
			case <-ec.HandshakeComplete():
				c = ec.NextConnection()
			}
			return c, nil
		}

		return &doqWithClose{
			u: doq.NewUpstream(dialer),
			t: t,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol [%s]", addrURL.Scheme)
	}
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

type doqWithClose struct {
	u *doq.Upstream
	t *quic.Transport
}

func (u *doqWithClose) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	return u.u.ExchangeContext(ctx, m)
}

func (u *doqWithClose) Close() error {
	return u.t.Close()
}

type dohWithClose struct {
	u      *doh.Upstream
	closer io.Closer // maybe nil
}

func (u *dohWithClose) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	return u.u.ExchangeContext(ctx, m)
}

func (u *dohWithClose) Close() error {
	if u.closer != nil {
		return u.closer.Close()
	}
	return nil
}

func newDefaultQuicConfig() *quic.Config {
	return &quic.Config{
		TokenStore: quic.NewLRUTokenStore(4, 8),

		// Dns does not need large amount of io, so the rx/tx windows are small.
		InitialStreamReceiveWindow:     4 * 1024,
		MaxStreamReceiveWindow:         4 * 1024,
		InitialConnectionReceiveWindow: 8 * 1024,
		MaxConnectionReceiveWindow:     64 * 1024,

		MaxIdleTimeout:       time.Second * 30,
		KeepAlivePeriod:      time.Second * 25,
		HandshakeIdleTimeout: tlsHandshakeTimeout,
	}
}
