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
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	dialTimeout         = time.Second * 5
	generalReadTimeout  = time.Second * 5
	generalWriteTimeout = time.Second * 1
	tlsHandshakeTimeout = time.Second * 5
)

type protocol uint8

const (
	protocolUDP protocol = iota
	protocolTCP
	protocolDoT
	protocolDoH
)

type FastUpstream struct {
	rawAddr string // original addr passed in NewFastUpstream()

	protocol    protocol
	dialAddr    string // Upstream "host:port" address to dial connections.
	altDialAddr string // set by WithDialAddr. Has higher priority than dialAddr.
	serverName  string // dot server name
	url         string // doh url

	socks5 string // socks5 server ip address. Used by tcp, dot, doh.

	// readTimeout
	// In "udp", "tcp", "dot", it's read timeout.
	// In "doh", it's a time limit for the query, including dialing connections.
	// Default is generalReadTimeout.
	readTimeout time.Duration

	// idleTimeout used by "tcp", "dot", "doh" to control connection idle timeout.
	// Default: "tcp" & "dot": 0 (disable connection reuse), "doh": 30s.
	idleTimeout time.Duration

	// maxConns limits the total number of connections,
	// including connections in the dialing states.
	// Used by "udp"( when falling back to tcp), "tcp", "dot", "doh". Default: 1.
	maxConns int

	rootCAs            *x509.CertPool
	insecureSkipVerify bool // Used by "dot", "doh". Skip tls verification.

	logger       *zap.Logger
	udpTransport *Transport   // used by udp
	tcpTransport *Transport   // used by udp(when falling back to tcp), tcp, dot.
	httpClient   *http.Client // used by doh.
}

func NewFastUpstream(addr string, options ...Option) (*FastUpstream, error) {
	u := new(FastUpstream)
	u.rawAddr = addr

	// parse protocol and server addr
	if !strings.Contains(addr, "://") {
		addr = "udp://" + addr
	}
	addrURL, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid server address, %w", err)
	}
	protocol, ok := parseScheme(addrURL.Scheme)
	if !ok {
		return nil, fmt.Errorf("invalid scheme [%s]", addrURL.Scheme)
	}
	u.protocol = protocol

	switch protocol {
	case protocolUDP, protocolTCP:
		u.dialAddr = tryAddDefaultPort(addrURL.Host, 53)
	case protocolDoT:
		u.dialAddr = tryAddDefaultPort(addrURL.Host, 853)
		u.serverName = addrURL.Hostname()
	case protocolDoH:
		u.dialAddr = tryAddDefaultPort(addrURL.Host, 443)
		u.url = addrURL.String()
	default:
		panic(fmt.Sprintf("unexpected protocol %d", protocol))
	}

	// apply options
	for _, op := range options {
		if err := op(u); err != nil {
			return nil, err
		}
	}

	// logger can not be nil
	if u.logger == nil {
		u.logger = zap.NewNop()
	}

	// udpTransport
	if u.protocol == protocolUDP {
		u.udpTransport = &Transport{
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
			Timeout:     u.getReadTimeout(),
		}
	}

	// tcpTransport
	if u.protocol == protocolUDP || u.protocol == protocolTCP || u.protocol == protocolDoT {
		var dialFunc func() (net.Conn, error)
		if u.protocol == protocolDoT {
			tlsConfig := new(tls.Config)
			tlsConfig.ServerName = u.serverName
			tlsConfig.RootCAs = u.rootCAs
			tlsConfig.InsecureSkipVerify = u.insecureSkipVerify

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

		u.tcpTransport = &Transport{
			Logger:      u.logger,
			DialFunc:    dialFunc,
			WriteFunc:   dnsutils.WriteMsgToTCP,
			ReadFunc:    dnsutils.ReadMsgFromTCP,
			MaxConns:    u.getMaxConns(),
			IdleTimeout: u.getIdleTimeout(),
			Timeout:     u.getReadTimeout(),
		}
	}

	// httpClient
	if u.protocol == protocolDoH {
		t := &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
				return u.dialContext(ctx, network)
			},
			TLSClientConfig: &tls.Config{
				RootCAs:            u.rootCAs,
				InsecureSkipVerify: u.insecureSkipVerify,
			},
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			IdleConnTimeout:       u.getIdleTimeout(),
			ResponseHeaderTimeout: u.getReadTimeout(),
			// MaxConnsPerHost and MaxIdleConnsPerHost should be equal.
			// Otherwise, it might seriously affect the efficiency of connection reuse.
			MaxConnsPerHost:     u.getMaxConns(),
			MaxIdleConnsPerHost: u.getMaxConns(),
		}
		t2, err := http2.ConfigureTransports(t)
		if err != nil {
			u.logger.Error("http2.ConfigureTransports", zap.Error(err))
		}

		if t2 != nil {
			t2.ReadIdleTimeout = time.Second * 30
			t2.PingTimeout = time.Second * 5
		}

		u.httpClient = &http.Client{
			Transport: t,
		}
	}

	return u, nil
}

func tryAddDefaultPort(addr string, port int) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil { // no port, add it.
		return net.JoinHostPort(addr, strconv.Itoa(port))
	}
	return addr
}

func parseScheme(s string) (protocol, bool) {
	switch s {
	case "", "udp":
		return protocolUDP, true
	case "tcp":
		return protocolTCP, true
	case "tls":
		return protocolDoT, true
	case "https":
		return protocolDoH, true
	default:
		return 0, false
	}
}

func (u *FastUpstream) getReadTimeout() time.Duration {
	if d := u.readTimeout; d > 0 {
		return d
	}
	return generalReadTimeout
}

func (u *FastUpstream) getIdleTimeout() time.Duration {
	if d := u.idleTimeout; d > 0 {
		return d
	}
	if u.protocol == protocolDoH {
		return time.Second * 30
	}
	return 0
}

func (u *FastUpstream) getMaxConns() int {
	if n := u.maxConns; n > 0 {
		return n
	}
	return 1
}

func (u *FastUpstream) Address() string {
	return u.rawAddr
}

func (u *FastUpstream) Exchange(q *dns.Msg) (r *dns.Msg, err error) {
	return u.exchange(q, false)
}

func (u *FastUpstream) ExchangeNoTruncated(q *dns.Msg) (r *dns.Msg, err error) {
	return u.exchange(q, true)
}

func (u *FastUpstream) exchange(q *dns.Msg, noTruncated bool) (*dns.Msg, error) {
	var r *dns.Msg
	var err error
	switch u.protocol {
	case protocolUDP:
		r, err = u.exchangeUDP(q, noTruncated)
	case protocolTCP, protocolDoT:
		r, err = u.exchangeTCP(q)
	case protocolDoH:
		r, err = u.exchangeDoH(q)
	default:
		err = fmt.Errorf("fastUpstream: invalid protocol %d", u.protocol)
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

// dialContext connects to the server.
// If network is "tcp", "tcp4", "tcp6", and FastUpstream.socks5 is not empty, it will
// dial through the socks5 server.
func (u *FastUpstream) dialContext(ctx context.Context, network string) (net.Conn, error) {
	var addr string
	if len(u.altDialAddr) != 0 {
		addr = u.altDialAddr
	} else {
		addr = u.dialAddr
	}

	if len(u.socks5) != 0 {
		switch network {
		case "tcp", "tcp4", "tcp6":
			socks5Dialer, err := proxy.SOCKS5("tcp", u.socks5, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
			}
			return socks5Dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", addr)
		}
	}

	d := net.Dialer{}
	return d.DialContext(ctx, network, addr)
}

func (u *FastUpstream) exchangeTCP(q *dns.Msg) (r *dns.Msg, err error) {
	return u.tcpTransport.Exchange(q)
}

func (u *FastUpstream) exchangeUDP(q *dns.Msg, noTruncated bool) (r *dns.Msg, err error) {
	r, err = u.udpTransport.Exchange(q)
	if err != nil {
		return nil, err
	}
	if r.Truncated && noTruncated { // fallback to tcp
		return u.exchangeTCP(q)
	}
	return r, nil
}
