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

package fastforward

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

// fastUpstream is a udp, tcp, dot, doh upstream
type fastUpstream struct {
	logger *zap.Logger
	config *UpstreamConfig

	protocol    dnsProtocol
	readTimeout time.Duration

	// upstream address
	address string

	udpTransport *transport // used by udp
	tcpTransport *transport // used by udp(upgraded), tcp, dot.

	httpClient *http.Client // used by doh.
}

type dnsProtocol uint8

const (
	protocolUDP dnsProtocol = iota
	protocolTCP
	protocolDoT
	protocolDoH
)

func newFastUpstream(config *UpstreamConfig, logger *zap.Logger, certPool *x509.CertPool) (*fastUpstream, error) {
	if len(config.Addr) == 0 { // Addr should never be empty
		return nil, errors.New("empty upstream addr")
	}

	timeout := generalReadTimeout
	if config.Timeout > 0 {
		timeout = time.Second * time.Duration(config.Timeout)
	}
	maxConn := 1
	if config.MaxConns != 0 {
		maxConn = int(config.MaxConns)
	}

	// pares protocol and idleTimeout, add default port, check config.
	var (
		idleTimeout time.Duration // used by tcp, dot, doh
		protocol    dnsProtocol
		address     string
	)
	switch config.Protocol {
	case "", "udp":
		protocol = protocolUDP
		config.Addr = utils.TryAddPort(config.Addr, 53)
		address = "udp://" + config.Addr
		idleTimeout = time.Second * 0 // this is for tcp fallback
	case "tcp":
		protocol = protocolTCP
		config.Addr = utils.TryAddPort(config.Addr, 53)
		address = "tcp://" + config.Addr
		idleTimeout = time.Second * 0
	case "dot", "tls":
		if len(config.ServerName) == 0 {
			return nil, errors.New("dot server name is empty")
		}
		protocol = protocolDoT
		config.Addr = utils.TryAddPort(config.Addr, 853)
		address = "tls://" + config.ServerName
		idleTimeout = time.Second * 0
	case "doh", "https":
		if len(config.URL) == 0 {
			return nil, errors.New("doh server url is empty")
		}
		protocol = protocolDoH
		config.Addr = utils.TryAddPort(config.Addr, 443)
		address = config.URL
		idleTimeout = time.Second * 30
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", config.Protocol)
	}

	if config.IdleTimeout != 0 { // overwrite default idle timeout
		idleTimeout = time.Second * time.Duration(config.IdleTimeout)
	}

	// init other stuffs in u

	u := &fastUpstream{
		logger:      logger.With(zap.String("addr", address)),
		config:      config,
		protocol:    protocol,
		readTimeout: timeout,
		address:     address,

		// init them later
		udpTransport: nil,
		tcpTransport: nil,
		httpClient:   nil,
	}

	// udpTransport
	if protocol == protocolUDP {
		logger := logger.With(zap.String("addr", address))
		u.udpTransport = newTransport(
			logger,
			func() (net.Conn, error) {
				return u.dialTimeout("udp", dialTimeout)
			},
			dnsutils.WriteMsgToUDP,
			func(c io.Reader) (m *dns.Msg, n int, err error) {
				return dnsutils.ReadMsgFromUDP(c, dnsutils.IPv4UdpMaxPayload)
			},
			maxConn,
			time.Second*30,
			timeout,
		)
	}

	// tcpTransport
	if protocol == protocolUDP || protocol == protocolTCP || protocol == protocolDoT {
		logger := logger.With(zap.String("addr", address))
		var dialFunc func() (net.Conn, error)
		if protocol == protocolDoT {
			tlsConfig := new(tls.Config)
			tlsConfig.ServerName = config.ServerName
			tlsConfig.RootCAs = certPool
			tlsConfig.InsecureSkipVerify = config.InsecureSkipVerify

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
			logger,
			dialFunc,
			dnsutils.WriteMsgToTCP,
			dnsutils.ReadMsgFromTCP,
			maxConn,
			idleTimeout,
			timeout,
		)
	}

	// httpClient
	if protocol == protocolDoH {
		t := &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
				return u.dialContext(ctx, network)
			},
			TLSClientConfig: &tls.Config{
				RootCAs:            certPool,
				InsecureSkipVerify: config.InsecureSkipVerify,
			},
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			IdleConnTimeout:       idleTimeout,
			ResponseHeaderTimeout: timeout,
			// MaxConnsPerHost and MaxIdleConnsPerHost should be equal.
			// Otherwise, it might seriously affect the efficiency of connection reuse.
			MaxConnsPerHost:     maxConn,
			MaxIdleConnsPerHost: maxConn,
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

func (u *fastUpstream) Address() string {
	return u.address
}

func (u *fastUpstream) Trusted() bool {
	return u.config.Trusted
}

func (u *fastUpstream) Exchange(qCtx *handler.Context) (r *dns.Msg, err error) {
	q := qCtx.Q()
	switch u.protocol {
	case protocolUDP:
		if qCtx.IsTCPClient() { // upgrade to tcp
			return u.exchangeTCP(q)
		}
		return u.exchangeUDP(q)
	case protocolTCP, protocolDoT:
		return u.exchangeTCP(q)
	case protocolDoH:
		return u.exchangeDoH(q)
	default:
		panic(fmt.Sprintf("fastUpstream: invalid protocol %d", u.protocol))
	}
}

// dialTimeout: see dialContext.
func (u *fastUpstream) dialTimeout(network string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return u.dialContext(ctx, network)
}

// dialContext dials a connection.
// If network is "tcp", "tcp4", "tcp6", and UpstreamConfig.Socks5 is not empty, it will
// dial through the socks5 server.
func (u *fastUpstream) dialContext(ctx context.Context, network string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
		if len(u.config.Socks5) != 0 {
			socks5Dialer, err := proxy.SOCKS5("tcp", u.config.Socks5, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
			}

			return socks5Dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", u.config.Addr)
		}
	}

	d := net.Dialer{}
	return d.DialContext(ctx, network, u.config.Addr)
}

func (u *fastUpstream) exchangeTCP(q *dns.Msg) (r *dns.Msg, err error) {
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

func (u *fastUpstream) exchangeUDP(q *dns.Msg) (r *dns.Msg, err error) {
	r, _, err = u.udpTransport.exchange(q)
	return r, err
}
