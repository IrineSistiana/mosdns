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
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/fast_forward/cpool"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
	"net"
	"net/http"
	"time"
)

// fastUpstream is a udp, tcp, dot upstream
type fastUpstream struct {
	logger *zap.Logger
	config *UpstreamConfig

	protocol    dnsProtocol
	readTimeout time.Duration

	// upstream address
	address string

	// Connection pool. If they are nil, connections won't be reused.
	udpPool *cpool.Pool // used by udp
	tcpPool *cpool.Pool // used by udp, tcp, dot.

	tlsConfig *tls.Config // Used by dot. It cannot be nil.

	httpClient *http.Client // Used by doh. It cannot be nil.
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

	// udpPool
	var udpPool *cpool.Pool
	if protocol == protocolUDP && config.IdleTimeout > 0 {
		poolLogger := logger.With(zap.String("addr", address), zap.String("protocol", "udp"))
		udpPool = cpool.New(0xffff, time.Second*30, time.Second*15, poolLogger)
	}

	// tcpPool
	var tcpPool *cpool.Pool
	if (protocol == protocolUDP || protocol == protocolTCP || protocol == protocolDoT) && config.IdleTimeout > 0 {
		poolLogger := logger.With(zap.String("addr", address), zap.String("protocol", "tcp"))
		tcpPool = cpool.New(0xffff, idleTimeout, time.Second*2, poolLogger)
	}

	// tlsConfig
	var tlsConfig *tls.Config
	if protocol == protocolDoT {
		tlsConfig = new(tls.Config)
		tlsConfig.ServerName = config.ServerName
		tlsConfig.RootCAs = certPool
		tlsConfig.InsecureSkipVerify = config.InsecureSkipVerify
	}

	u := &fastUpstream{
		logger:      logger.With(zap.String("addr", address)),
		config:      config,
		protocol:    protocol,
		readTimeout: timeout,
		address:     address,
		udpPool:     udpPool,
		tcpPool:     tcpPool,
		tlsConfig:   tlsConfig,
		httpClient:  nil, // init it later
	}

	// httpClient
	if protocol == protocolDoH {
		maxConn := 1
		if config.MaxConns != 0 {
			maxConn = int(config.MaxConns)
		}

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
		if qCtx.IsTCPClient() { // force upgrade to tcp
			return u.exchangeTCP(q, u.dialTCP)
		}
		return u.exchangeUDP(q)
	case protocolTCP:
		return u.exchangeTCP(q, u.dialTCP)
	case protocolDoT:
		return u.exchangeTCP(q, u.dialTLS)
	case protocolDoH:
		return u.exchangeDoH(q)
	default:
		panic(fmt.Sprintf("fastUpstream: invalid protocol %d", u.protocol))
	}
}

func (u *fastUpstream) dialTCP() (net.Conn, error) {
	return u.dialTimeout("tcp", dialTimeout)
}

func (u *fastUpstream) dialTLS() (net.Conn, error) {
	c, err := u.dialTimeout("tcp", dialTimeout)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(c, u.tlsConfig)
	c.SetDeadline(time.Now().Add(tlsHandshakeTimeout))
	if err := tlsConn.Handshake(); err != nil {
		c.Close()
		return nil, err
	}
	c.SetDeadline(time.Time{})
	return tlsConn, nil
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

func (u *fastUpstream) exchangeTCP(q *dns.Msg, dialFunc func() (net.Conn, error)) (r *dns.Msg, err error) {
	if u.tcpPool != nil {
		return u.exchangeTCPWithPool(q, dialFunc)
	}

	c, err := dialFunc()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return u.exchangeViaTCPConn(q, c)
}

func (u *fastUpstream) exchangeTCPWithPool(q *dns.Msg, dialFunc func() (net.Conn, error)) (r *dns.Msg, err error) {
	c := u.tcpPool.Get()
	start := time.Now()

exchange:
	var isNewConn bool
	if c == nil {
		c, err = dialFunc()
		if err != nil {
			return nil, err
		}
		isNewConn = true
	}

	r, err = u.exchangeViaTCPConn(q, c)
	if err != nil {
		c.Close()
		if !isNewConn && time.Now().Sub(start) < time.Millisecond*100 {
			// There has a race condition between client and server.
			// If reused connection returned an err very quickly,
			// Dail a new connection and try again.
			c = nil
			goto exchange
		} else {
			return nil, err
		}
	}

	u.tcpPool.Put(c)
	return r, nil
}

func (u *fastUpstream) exchangeViaTCPConn(q *dns.Msg, c net.Conn) (r *dns.Msg, err error) {
	c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err = utils.WriteMsgToTCP(c, q)
	if err != nil {
		return nil, err
	}
	c.SetReadDeadline(time.Now().Add(u.readTimeout))
	r, _, err = utils.ReadMsgFromTCP(c)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (u *fastUpstream) exchangeUDP(q *dns.Msg) (r *dns.Msg, err error) {
	if u.udpPool != nil {
		return u.exchangeUDPWithPool(q)
	}

	dialer := net.Dialer{Timeout: dialTimeout}
	c, err := dialer.Dial("udp", u.config.Addr)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return u.exchangeViaUDPConn(q, c, false)
}

func (u *fastUpstream) exchangeUDPWithPool(q *dns.Msg) (r *dns.Msg, err error) {
	c := u.udpPool.Get()
	isNewConn := false
	if c == nil {
		dialer := net.Dialer{Timeout: dialTimeout}
		c, err = dialer.Dial("udp", u.config.Addr)
		if err != nil {
			return nil, err
		}
		isNewConn = true
	}

	r, err = u.exchangeViaUDPConn(q, c, isNewConn)
	if err != nil {
		c.Close()
		return nil, err
	}

	u.udpPool.Put(c)
	return r, nil
}

func (u *fastUpstream) exchangeViaUDPConn(q *dns.Msg, c net.Conn, isNewConn bool) (r *dns.Msg, err error) {
	c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err = utils.WriteMsgToUDP(c, q)
	if err != nil { // write err typically is a fatal err
		return nil, err
	}
	c.SetReadDeadline(time.Now().Add(u.readTimeout))

	if isNewConn {
		r, _, err = utils.ReadMsgFromUDP(c, utils.IPv4UdpMaxPayload)
		return r, err
	} else {

		// Reused udp sockets might have dirty data in the read buffer,
		// say, multi-package dns pollution.
		// This is a simple workaround.
		for {
			r, _, err = utils.ReadMsgFromUDP(c, utils.IPv4UdpMaxPayload)
			if err != nil {
				return nil, err
			}

			// id mismatch, ignore it and read again.
			if r.Id != q.Id {
				continue
			}
			return r, nil
		}
	}
}
