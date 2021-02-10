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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	dialTimeout         = time.Second * 5
	generalReadTimeout  = time.Second * 5
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

type Option struct {
	Logger *zap.Logger // Nil value disables the logger.

	// Addr is a network "host:port" addr, ":port" can be omitted.
	// Addr cannot be empty.
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
}

// FastUpstream is a udp, tcp, dot, doh upstream
type FastUpstream struct {
	protocol   Protocol
	socks5Addr string
	serverName string
	url        string

	logger       *zap.Logger
	maxConns     int
	readTimeout  time.Duration
	addrWithPort string
	idleTimeout  time.Duration

	udpTransport *transport   // used by udp
	tcpTransport *transport   // used by udp(upgraded), tcp, dot.
	httpClient   *http.Client // used by doh.
}

func NewUpstream(opt *Option) (*FastUpstream, error) {
	if len(opt.Addr) == 0 {
		return nil, errors.New("empty server addr")
	}

	u := new(FastUpstream)
	u.protocol = opt.Protocol

	switch opt.Protocol {
	case ProtocolUDP:
		u.addrWithPort = utils.TryAddPort(opt.Addr, 53)
		u.idleTimeout = time.Second * 0 // this is for tcp fallback
	case ProtocolTCP:
		u.addrWithPort = utils.TryAddPort(opt.Addr, 53)
		u.idleTimeout = time.Second * 0
	case ProtocolDoT:
		if len(opt.ServerName) == 0 {
			return nil, errors.New("missing arg for dot protocol: server name")
		}
		u.addrWithPort = utils.TryAddPort(opt.Addr, 853)
		u.serverName = opt.ServerName
		u.idleTimeout = time.Second * 0
	case ProtocolDoH:
		if len(opt.URL) == 0 {
			return nil, errors.New("missing arg for doh protocol: url")
		}
		u.addrWithPort = utils.TryAddPort(opt.Addr, 443)
		u.url = opt.URL
		u.idleTimeout = time.Second * 30
	}

	if opt.Logger != nil {
		u.logger = opt.Logger
	} else {
		u.logger = zap.NewNop()
	}

	u.readTimeout = generalReadTimeout
	if opt.ReadTimeout > 0 {
		u.readTimeout = opt.ReadTimeout
	}

	u.maxConns = 1
	if opt.MaxConns > 0 {
		u.maxConns = opt.MaxConns
	}

	if opt.IdleTimeout > 0 { // overwrite default idle timeout
		u.idleTimeout = opt.IdleTimeout
	}

	// init other stuffs in u

	// udpTransport
	if opt.Protocol == ProtocolUDP {
		u.udpTransport = newTransport(
			u.logger,
			func() (net.Conn, error) {
				return u.dialTimeout("udp", dialTimeout)
			},
			dnsutils.WriteMsgToUDP,
			func(c io.Reader) (m *dns.Msg, n int, err error) {
				return dnsutils.ReadMsgFromUDP(c, dnsutils.IPv4UdpMaxPayload)
			},
			u.maxConns,
			time.Second*30,
			u.readTimeout,
		)
	}

	// tcpTransport
	if opt.Protocol == ProtocolUDP || opt.Protocol == ProtocolTCP || opt.Protocol == ProtocolDoT {
		var dialFunc func() (net.Conn, error)
		if opt.Protocol == ProtocolDoT {
			tlsConfig := new(tls.Config)
			tlsConfig.ServerName = opt.ServerName
			tlsConfig.RootCAs = opt.RootCA
			tlsConfig.InsecureSkipVerify = opt.InsecureSkipVerify

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

		u.tcpTransport = newTransport(
			u.logger,
			dialFunc,
			dnsutils.WriteMsgToTCP,
			dnsutils.ReadMsgFromTCP,
			u.maxConns,
			u.idleTimeout,
			u.readTimeout,
		)
	}

	// httpClient
	if opt.Protocol == ProtocolDoH {
		t := &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
				return u.dialContext(ctx, network)
			},
			TLSClientConfig: &tls.Config{
				RootCAs:            opt.RootCA,
				InsecureSkipVerify: opt.InsecureSkipVerify,
			},
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			IdleConnTimeout:       u.idleTimeout,
			ResponseHeaderTimeout: u.readTimeout,
			// MaxConnsPerHost and MaxIdleConnsPerHost should be equal.
			// Otherwise, it might seriously affect the efficiency of connection reuse.
			MaxConnsPerHost:     u.maxConns,
			MaxIdleConnsPerHost: u.maxConns,
		}
		t2, err := http2.ConfigureTransports(t)
		if err != nil {
			return nil, err
		}

		t2.ReadIdleTimeout = time.Second * 30
		t2.PingTimeout = time.Second * 5

		u.httpClient = &http.Client{
			Transport: t,
		}
	}

	return u, nil
}

func (u *FastUpstream) Exchange(qCtx *handler.Context) (r *dns.Msg, err error) {
	q := qCtx.Q()
	switch u.protocol {
	case ProtocolUDP:
		if qCtx.IsTCPClient() { // upgrade to tcp
			return u.exchangeTCP(q)
		}
		return u.exchangeUDP(q)
	case ProtocolTCP, ProtocolDoT:
		return u.exchangeTCP(q)
	case ProtocolDoH:
		return u.exchangeDoH(q)
	default:
		panic(fmt.Sprintf("fastUpstream: invalid protocol %d", u.protocol))
	}
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
		if len(u.socks5Addr) != 0 {
			socks5Dialer, err := proxy.SOCKS5("tcp", u.socks5Addr, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
			}

			return socks5Dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", u.addrWithPort)
		}
	}

	d := net.Dialer{}
	return d.DialContext(ctx, network, u.addrWithPort)
}

func (u *FastUpstream) exchangeTCP(q *dns.Msg) (r *dns.Msg, err error) {
	start := time.Now()
	retry := 0
exchangeAgain:
	r, reusedConn, err := u.tcpTransport.exchange(q)
	if err != nil && reusedConn == true && retry < 3 && time.Since(start) < time.Millisecond*100 {
		retry++
		goto exchangeAgain
	}
	return r, err
}

func (u *FastUpstream) exchangeUDP(q *dns.Msg) (r *dns.Msg, err error) {
	r, _, err = u.udpTransport.exchange(q)
	return r, err
}
