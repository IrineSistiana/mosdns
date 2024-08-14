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
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/bootstrap"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/doh"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/transport"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

const (
	tlsHandshakeTimeout = time.Second * 3

	// Maximum number of concurrent queries in one pipeline connection.
	// See RFC 7766 7. Response Reordering.
	// TODO: Make this configurable?
	pipelineConcurrentLimit = 64
)

// Upstream represents a DNS upstream.
type Upstream interface {
	// ExchangeContext exchanges query message m to the upstream, and returns
	// response. It MUST NOT keep or modify m.
	// m MUST be a valid dns msg frame. It MUST be at least 12 bytes
	// (contain a valid dns header).
	ExchangeContext(ctx context.Context, m []byte) (*[]byte, error)

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
	// Default: TCP, DoT: 10s , DoH, DoH3, Quic: 30s.
	IdleTimeout time.Duration

	// EnablePipeline enables query pipelining support as RFC 7766 6.2.1.1 suggested.
	// Available for TCP, DoT upstream.
	// Note: There is no fallback. Make sure the server supports it.
	EnablePipeline bool

	// EnableHTTP3 will use HTTP/3 protocol to connect a DoH upstream. (aka DoH3).
	// Note: There is no fallback. Make sure the server supports it.
	EnableHTTP3 bool

	// Bootstrap specifies a plain dns server to solve the
	// upstream server domain address.
	// It must be an IP address. Port is optional.
	Bootstrap string

	// Bootstrap version. One of 0 (default equals 4), 4, 6.
	// TODO: Support dual-stack.
	BootstrapVer int

	// TLSConfig specifies the tls.Config that the TLS client will use.
	// Available for DoT, DoH, DoQ upstream.
	TLSConfig *tls.Config

	// Logger specifies the logger that the upstream will use.
	Logger *zap.Logger

	// EventObserver can observe connection events.
	// Not implemented for quic based protocol (DoH3, DoQ).
	EventObserver EventObserver
}

// NewUpstream creates a upstream.
// addr has the format of: [protocol://]host[:port][/path].
// Supported protocol: udp/tcp/tls/https/quic. Default protocol is udp.
//
// Helper protocol:
//   - tcp+pipeline/tls+pipeline: Automatically set opt.EnablePipeline to true.
//   - h3: Automatically set opt.EnableHTTP3 to true.
func NewUpstream(addr string, opt Opt) (_ Upstream, err error) {
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

	// Apply helper protocol
	switch addrURL.Scheme {
	case "tcp+pipeline", "tls+pipeline":
		addrURL.Scheme = addrURL.Scheme[:3]
		opt.EnablePipeline = true
	case "h3":
		addrURL.Scheme = "https"
		opt.EnableHTTP3 = true
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

	closeIfFuncErr := func(c io.Closer) {
		if err != nil {
			c.Close()
		}
	}

	switch addrURL.Scheme {
	case "", "udp":
		const defaultPort = 53
		const maxConcurrentQueryPreConn = 4096 // Protocol limit is 65535.
		host, port, err := parseDialAddr(addrUrlHost, opt.DialAddr, defaultPort)
		if err != nil {
			return nil, err
		}
		if _, err := netip.ParseAddr(host); err != nil {
			return nil, fmt.Errorf("addr must be an ip address, %w", err)
		}
		dialAddr := joinPort(host, port)

		dialUdpPipeline := func(ctx context.Context) (transport.DnsConn, error) {
			c, err := dialer.DialContext(ctx, "udp", dialAddr)
			if err != nil {
				return nil, err
			}
			to := transport.TraditionalDnsConnOpts{
				WithLengthHeader:   false,
				IdleTimeout:        time.Minute * 5,
				MaxConcurrentQuery: maxConcurrentQueryPreConn,
			}
			return transport.NewDnsConn(to, wrapConn(c, opt.EventObserver)), nil
		}
		dialTcpNetConn := func(ctx context.Context) (transport.NetConn, error) {
			c, err := dialer.DialContext(ctx, "tcp", dialAddr)
			if err != nil {
				return nil, err
			}
			return wrapConn(c, opt.EventObserver), nil
		}

		return &udpWithFallback{
			u: transport.NewPipelineTransport(transport.PipelineOpts{
				DialContext:                    dialUdpPipeline,
				MaxConcurrentQueryWhileDialing: maxConcurrentQueryPreConn,
				Logger:                         opt.Logger,
			}),
			t: transport.NewReuseConnTransport(transport.ReuseConnOpts{DialContext: dialTcpNetConn}),
		}, nil
	case "tcp":
		const defaultPort = 53
		tcpDialer, err := newTcpDialer(true, defaultPort)
		if err != nil {
			return nil, fmt.Errorf("failed to init tcp dialer, %w", err)
		}
		idleTimeout := opt.IdleTimeout
		if idleTimeout <= 0 {
			idleTimeout = time.Second * 10
		}

		dialNetConn := func(ctx context.Context) (transport.NetConn, error) {
			c, err := tcpDialer(ctx)
			if err != nil {
				return nil, err
			}
			return wrapConn(c, opt.EventObserver), nil
		}
		if opt.EnablePipeline {
			to := transport.TraditionalDnsConnOpts{
				WithLengthHeader:   true,
				IdleTimeout:        idleTimeout,
				MaxConcurrentQuery: pipelineConcurrentLimit,
			}
			dialDnsConn := func(ctx context.Context) (transport.DnsConn, error) {
				c, err := dialNetConn(ctx)
				if err != nil {
					return nil, err
				}
				return transport.NewDnsConn(to, c), nil
			}
			return transport.NewPipelineTransport(transport.PipelineOpts{
				DialContext:                    dialDnsConn,
				MaxConcurrentQueryWhileDialing: pipelineConcurrentLimit,
				Logger:                         opt.Logger,
			}), nil
		}
		return transport.NewReuseConnTransport(transport.ReuseConnOpts{DialContext: dialNetConn, IdleTimeout: idleTimeout}), nil
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

		dialNetConn := func(ctx context.Context) (transport.NetConn, error) {
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
			return wrapConn(tlsConn, opt.EventObserver), nil
		}

		if opt.EnablePipeline {
			to := transport.TraditionalDnsConnOpts{
				WithLengthHeader:   true,
				IdleTimeout:        opt.IdleTimeout,
				MaxConcurrentQuery: pipelineConcurrentLimit,
			}
			dialDnsConn := func(ctx context.Context) (transport.DnsConn, error) {
				c, err := dialNetConn(ctx)
				if err != nil {
					return nil, err
				}
				return transport.NewDnsConn(to, c), nil
			}
			return transport.NewPipelineTransport(transport.PipelineOpts{
				DialContext:                    dialDnsConn,
				MaxConcurrentQueryWhileDialing: pipelineConcurrentLimit,
				Logger:                         opt.Logger,
			}), nil
		}
		return transport.NewReuseConnTransport(transport.ReuseConnOpts{DialContext: dialNetConn}), nil
	case "https":
		const defaultPort = 443

		idleConnTimeout := time.Second * 30
		if opt.IdleTimeout > 0 {
			idleConnTimeout = opt.IdleTimeout
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
				return nil, fmt.Errorf("failed to init udp socket for quic, %w", err)
			}
			quicTransport := &quic.Transport{
				Conn: conn,
			}
			quicConfig := newDefaultClientQuicConfig()
			quicConfig.MaxIdleTimeout = idleConnTimeout

			defer closeIfFuncErr(quicTransport)
			addonCloser = quicTransport
			t = &http3.RoundTripper{
				TLSClientConfig: opt.TLSConfig,
				QUICConfig:      quicConfig,
				Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
					ua, err := udpBootstrap(ctx)
					if err != nil {
						return nil, err
					}
					return quicTransport.DialEarly(ctx, ua, tlsCfg, cfg)
				},
				MaxResponseHeaderBytes: 4 * 1024,
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

				// Following opts are for http/1 only.
				// MaxConnsPerHost:     2,
				// MaxIdleConnsPerHost: 2,
			}

			t2, err := http2.ConfigureTransports(t1)
			if err != nil {
				return nil, fmt.Errorf("failed to upgrade http2 support, %w", err)
			}
			t2.MaxHeaderListSize = 4 * 1024
			t2.MaxReadFrameSize = 16 * 1024
			t2.ReadIdleTimeout = time.Second * 30
			t2.PingTimeout = time.Second * 5
			t = t1
		}

		u, err := doh.NewUpstream(addrURL.String(), t, opt.Logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create doh upstream, %w", err)
		}

		return &dohWithClose{
			u:      u,
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
		tlsConfig.NextProtos = []string{"doq"}

		quicConfig := newDefaultClientQuicConfig()
		if opt.IdleTimeout > 0 {
			quicConfig.MaxIdleTimeout = opt.IdleTimeout
		}
		// Don't accept stream.
		quicConfig.MaxIncomingStreams = -1
		quicConfig.MaxIncomingUniStreams = -1

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
			return nil, fmt.Errorf("failed to init udp socket for quic, %w", err)
		}

		t := &quic.Transport{
			Conn:              uc,
			StatelessResetKey: (*quic.StatelessResetKey)(srk),
		}

		dialDnsConn := func(ctx context.Context) (transport.DnsConn, error) {
			ua, err := udpBootstrap(ctx)
			if err != nil {
				return nil, fmt.Errorf("bootstrap failed, %w", err)
			}

			// This is a workaround to
			// 1. recover from strange 0rtt rejected err.
			// 2. avoid NextConnection might block forever.
			// TODO: Remove this workaround.
			var c quic.Connection
			ec, err := t.DialEarly(ctx, ua, tlsConfig, quicConfig)
			if err != nil {
				return nil, err
			}
			c, err = ec.NextConnection(ctx)
			if err != nil {
				return nil, err
			}
			return transport.NewQuicDnsConn(c), nil
		}

		return transport.NewPipelineTransport(transport.PipelineOpts{
			DialContext: dialDnsConn,
			// Quic rfc recommendation is 100. Some implications use 65535.
			MaxConcurrentQueryWhileDialing: 90,
			Logger:                         opt.Logger,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported protocol [%s]", addrURL.Scheme)
	}
}

type udpWithFallback struct {
	u *transport.PipelineTransport
	t *transport.ReuseConnTransport
}

func (u *udpWithFallback) ExchangeContext(ctx context.Context, q []byte) (*[]byte, error) {
	r, err := u.u.ExchangeContext(ctx, q)
	if err != nil {
		return nil, err
	}
	if msgTruncated(*r) {
		pool.ReleaseBuf(r)
		return u.t.ExchangeContext(ctx, q)
	}
	return r, nil
}

func (u *udpWithFallback) Close() error {
	u.u.Close()
	u.t.Close()
	return nil
}

type dohWithClose struct {
	u      *doh.Upstream
	closer io.Closer // maybe nil
}

func (u *dohWithClose) ExchangeContext(ctx context.Context, m []byte) (*[]byte, error) {
	return u.u.ExchangeContext(ctx, m)
}

func (u *dohWithClose) Close() error {
	if u.closer != nil {
		return u.closer.Close()
	}
	return nil
}

func newDefaultClientQuicConfig() *quic.Config {
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
