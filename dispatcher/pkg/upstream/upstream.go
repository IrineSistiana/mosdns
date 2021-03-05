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
	"crypto/x509"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	dialTimeout         = time.Second * 5
	generalReadTimeout  = time.Second * 5
	generalWriteTimeout = time.Second * 1
	tlsHandshakeTimeout = time.Second * 5
)

type Protocol uint8

// DNS protocols
const (
	ProtocolUDP Protocol = iota
	ProtocolTCP
	ProtocolDoT
	ProtocolDoH
)

// FastUpstream is a udp, tcp, dot, doh upstream
type FastUpstream struct {
	Logger *zap.Logger // Nil value disables the logger.

	// Addr is a network "host:port" addr
	Addr       string
	Protocol   Protocol // Default is ProtocolUDP.
	Socks5     string   // Used by "tcp", "dot", "doh" as Socks5 server addr.
	ServerName string   // Used by "dot" as server certificate name. It cannot be empty.
	URL        string   // Used by "doh" as server endpoint url. It cannot be empty.

	// ReadTimeout
	// In "udp", "tcp", "dot", it's read timeout.
	// In "doh", it's a time limit for the query, including dialing connections.
	// Default is generalReadTimeout.
	ReadTimeout time.Duration

	// IdleTimeout used by "tcp", "dot", "doh" to control connection idle timeout.
	// Default: "tcp" & "dot": 0 (disable connection reuse), "doh": 30s.
	IdleTimeout time.Duration

	// MaxConns limits the total number of connections,
	// including connections in the dialing states.
	// Used by "udp", "tcp", "dot", "doh". Default: 1.
	MaxConns int

	RootCA             *x509.CertPool
	InsecureSkipVerify bool // Used by "dot", "doh". Skip tls verification.

	initOnce     sync.Once
	logger       *zap.Logger  // non-nil logger
	udpTransport *Transport   // used by udp
	tcpTransport *Transport   // used by udp(upgraded), tcp, dot.
	httpClient   *http.Client // used by doh.
}

func (u *FastUpstream) readTimeout() time.Duration {
	if d := u.ReadTimeout; d > 0 {
		return d
	}
	return generalReadTimeout
}

func (u *FastUpstream) idleTimeout() time.Duration {
	if d := u.IdleTimeout; d > 0 {
		return d
	}
	switch u.Protocol {
	case ProtocolDoH:
		return time.Second * 30
	default:
		return 0
	}
}

func (u *FastUpstream) maxConns() int {
	if n := u.MaxConns; n > 0 {
		return n
	}
	return 1
}

func (u *FastUpstream) init() {
	if u.Logger != nil {
		u.logger = u.Logger
	} else {
		u.logger = zap.NewNop()
	}

	// udpTransport
	if u.Protocol == ProtocolUDP {
		udpTransport := &Transport{
			Logger: u.logger,
			DialFunc: func() (net.Conn, error) {
				return u.dialTimeout("udp", dialTimeout)
			},
			WriteFunc: dnsutils.WriteMsgToUDP,
			ReadFunc: func(c io.Reader) (m *dns.Msg, n int, err error) {
				return dnsutils.ReadMsgFromUDP(c, dnsutils.IPv4UdpMaxPayload)
			},
			MaxConns:    1,
			IdleTimeout: time.Second * 30,
			Timeout:     u.readTimeout(),
		}
		u.udpTransport = udpTransport
	}

	// tcpTransport
	if u.Protocol == ProtocolUDP || u.Protocol == ProtocolTCP || u.Protocol == ProtocolDoT {
		var dialFunc func() (net.Conn, error)
		if u.Protocol == ProtocolDoT {
			tlsConfig := new(tls.Config)
			tlsConfig.ServerName = u.ServerName
			tlsConfig.RootCAs = u.RootCA
			tlsConfig.InsecureSkipVerify = u.InsecureSkipVerify

			dialFunc = func() (net.Conn, error) {
				c, err := u.dialTimeout("tcp", dialTimeout)
				if err != nil {
					return nil, err
				}
				tlsConn := tls.Client(c, tlsConfig)
				c.SetDeadline(time.Now().Add(tlsHandshakeTimeout))
				if err := tlsConn.Handshake(); err != nil {
					c.Close()
					return nil, err
				}
				c.SetDeadline(time.Time{})
				return tlsConn, nil
			}
		} else {
			dialFunc = func() (net.Conn, error) {
				return u.dialTimeout("tcp", dialTimeout)
			}
		}

		tcpTransport := &Transport{
			Logger:      u.logger,
			DialFunc:    dialFunc,
			WriteFunc:   dnsutils.WriteMsgToTCP,
			ReadFunc:    dnsutils.ReadMsgFromTCP,
			MaxConns:    u.maxConns(),
			IdleTimeout: u.idleTimeout(),
			Timeout:     u.readTimeout(),
		}
		u.tcpTransport = tcpTransport
	}

	// httpClient
	if u.Protocol == ProtocolDoH {
		t := &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
				return u.dialContext(ctx, network)
			},
			TLSClientConfig: &tls.Config{
				RootCAs:            u.RootCA,
				InsecureSkipVerify: u.InsecureSkipVerify,
			},
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			IdleConnTimeout:       u.idleTimeout(),
			ResponseHeaderTimeout: u.readTimeout(),
			// MaxConnsPerHost and MaxIdleConnsPerHost should be equal.
			// Otherwise, it might seriously affect the efficiency of connection reuse.
			MaxConnsPerHost:     u.maxConns(),
			MaxIdleConnsPerHost: u.maxConns(),
		}
		t2, err := http2.ConfigureTransports(t)
		if err != nil {
			u.logger.Error("http2.ConfigureTransports", zap.Error(err))
		}

		if t2 != nil {
			t2.ReadIdleTimeout = time.Second * 30
			t2.PingTimeout = time.Second * 5
		}
		t.CloseIdleConnections()

		u.httpClient = &http.Client{
			Transport: t,
		}
	}
}

func (u *FastUpstream) Exchange(q *dns.Msg) (r *dns.Msg, err error) {
	return u.exchange(q, false)
}

func (u *FastUpstream) ExchangeNoTruncated(q *dns.Msg) (r *dns.Msg, err error) {
	return u.exchange(q, true)
}

func (u *FastUpstream) exchange(q *dns.Msg, noTruncated bool) (r *dns.Msg, err error) {
	u.initOnce.Do(u.init)

	switch u.Protocol {
	case ProtocolUDP:
		if noTruncated { // upgrade to tcp
			r, err = u.exchangeTCP(q)
		} else {
			r, err = u.exchangeUDP(q)
		}
	case ProtocolTCP, ProtocolDoT:
		r, err = u.exchangeTCP(q)
	case ProtocolDoH:
		r, err = u.exchangeDoH(q)
	default:
		err = fmt.Errorf("fastUpstream: invalid protocol %d", u.Protocol)
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// dialTimeout: see dialContext.
func (u *FastUpstream) dialTimeout(network string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return u.dialContext(ctx, network)
}

// dialContext dials a connection.
// If network is "tcp", "tcp4", "tcp6", and UpstreamConfig.Socks5 is not empty, it will
// dial through the socks5 server.
func (u *FastUpstream) dialContext(ctx context.Context, network string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
		if len(u.Socks5) != 0 {
			socks5Dialer, err := proxy.SOCKS5("tcp", u.Socks5, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
			}

			return socks5Dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", u.Addr)
		}
	}

	d := net.Dialer{}
	return d.DialContext(ctx, network, u.Addr)
}

func (u *FastUpstream) exchangeTCP(q *dns.Msg) (r *dns.Msg, err error) {
	return u.tcpTransport.Exchange(q)
}

func (u *FastUpstream) exchangeUDP(q *dns.Msg) (r *dns.Msg, err error) {
	return u.udpTransport.Exchange(q)
}
