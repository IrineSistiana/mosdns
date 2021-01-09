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

	mode    upstreamProtocol
	timeout time.Duration

	// upstream address
	address string

	// Connection pool. If they are nil, connections won't be reused.
	udpPool *cpool.Pool // used by udp
	tcpPool *cpool.Pool // used by udp, tcp, dot.

	// ca pool, used by dot, doh.
	certPool *x509.CertPool

	// used by tcp, dot to dial server connection. It cannot be nil.
	dialTCP func() (net.Conn, error)

	httpClient *http.Client // Used by doh. It cannot be nil.
}

type upstreamProtocol uint8

const (
	protocolUDP upstreamProtocol = iota
	protocolTCP
	protocolDoT
	protocolDoH
)

func newFastUpstream(config *UpstreamConfig, logger *zap.Logger) (*fastUpstream, error) {
	if len(config.Addr) == 0 { // Addr should never be empty
		return nil, errors.New("empty upstream addr")
	}

	var timeout time.Duration
	if config.Timeout > 0 {
		timeout = time.Second * time.Duration(config.Timeout)
	} else {
		timeout = generalReadTimeout
	}
	idleTimeout := time.Second * time.Duration(config.IdleTimeout)

	u := new(fastUpstream)

	// pares protocol and add port and check config.
	switch config.Protocol {
	case "", "udp":
		u.mode = protocolUDP
		config.Addr = utils.TryAddPort(config.Addr, 53)
		u.address = "udp://" + config.Addr
	case "tcp":
		u.mode = protocolTCP
		config.Addr = utils.TryAddPort(config.Addr, 53)
		u.address = "tcp://" + config.Addr
	case "dot":
		if len(config.ServerName) == 0 {
			return nil, errors.New("dot server name is empty")
		}
		u.mode = protocolDoT
		config.Addr = utils.TryAddPort(config.Addr, 853)
		u.address = "dot://" + config.ServerName
	case "doh", "https":
		if len(config.URL) == 0 {
			return nil, errors.New("doh server url is empty")
		}
		u.mode = protocolDoH
		config.Addr = utils.TryAddPort(config.Addr, 443)
		u.address = config.URL
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", config.Protocol)
	}

	// init other stuffs in u

	// udpPool
	if (u.mode == protocolUDP || u.mode == protocolTCP) && config.IdleTimeout > 0 {
		poolLogger := logger.With(zap.String("addr", config.Addr), zap.String("protocol", "udp"))
		u.udpPool = cpool.New(0xffff, idleTimeout, time.Second*2, poolLogger)
	}

	// tcpPool
	if (u.mode == protocolUDP || u.mode == protocolTCP || u.mode == protocolDoT) && config.IdleTimeout > 0 {
		poolLogger := logger.With(zap.String("addr", config.Addr), zap.String("protocol", "tcp"))
		u.tcpPool = cpool.New(0xffff, idleTimeout, time.Second*2, poolLogger)
	}

	// certPool
	if len(config.CA) != 0 {
		certPool, err := utils.LoadCertPool(config.CA)
		if err != nil {
			return nil, fmt.Errorf("failed to load ca: %w", err)
		}
		u.certPool = certPool
	}

	// dialContext
	if u.mode == protocolTCP {
		u.dialTCP = func() (net.Conn, error) {
			return u.dialTimeout("tcp", config.Addr, dialTimeout)
		}
	}
	if u.mode == protocolDoT {
		tlsConfig := new(tls.Config)
		tlsConfig.ServerName = config.ServerName
		tlsConfig.RootCAs = u.certPool
		tlsConfig.InsecureSkipVerify = config.InsecureSkipVerify

		u.dialTCP = func() (net.Conn, error) {
			c, err := u.dialTimeout("tcp", config.Addr, dialTimeout)
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
	}

	// httpClient
	if u.mode == protocolDoH {
		tlsConfig := new(tls.Config)
		tlsConfig.RootCAs = u.certPool
		tlsConfig.InsecureSkipVerify = config.InsecureSkipVerify
		t := &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) { // overwrite server addr
				return u.dialContext(ctx, network, config.Addr)
			},
			TLSClientConfig:       tlsConfig,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			DisableCompression:    true,
			DisableKeepAlives:     idleTimeout == 0,
			IdleConnTimeout:       idleTimeout,
			ResponseHeaderTimeout: timeout,
		}
		_, err := http2.ConfigureTransports(t)
		if err != nil {
			return nil, err
		}

		u.httpClient = &http.Client{
			Transport: t,
		}
	}

	u.logger = logger.With(zap.String("addr", u.address))
	u.config = config
	u.timeout = timeout
	return u, nil
}

func (u *fastUpstream) Address() string {
	return u.address
}

func (u *fastUpstream) Exchange(q *dns.Msg) (r *dns.Msg, err error) {
	switch u.mode {
	case protocolUDP:
		return u.exchangeUDPWithTCPFallback(q)
	case protocolTCP, protocolDoT:
		return u.exchangeTCP(q)
	case protocolDoH:
		return u.exchangeDoH(q)
	default:
		panic(fmt.Sprintf("fastUpstream: invalid mode %d", u.mode))
	}
}

// dialTimeout: see dialContext.
func (u *fastUpstream) dialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return u.dialContext(ctx, network, addr)
}

// dialContext dials a connection.
// If network is "tcp", "tcp4", "tcp6", and UpstreamConfig.Socks5 is not empty, it will
// dial through the socks5 server.
func (u *fastUpstream) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
		if len(u.config.Socks5) != 0 {
			socks5Dialer, err := proxy.SOCKS5("tcp", u.config.Socks5, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
			}

			return socks5Dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", addr)
		}
	}

	d := net.Dialer{}
	return d.DialContext(ctx, network, addr)
}

func (u *fastUpstream) exchangeTCP(q *dns.Msg) (r *dns.Msg, err error) {
	if u.tcpPool != nil {
		return u.exchangeTCPWithPool(q, u.tcpPool)
	}

	c, err := u.dialTCP()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return u.exchangeViaTCPConn(q, c)
}

func (u *fastUpstream) exchangeTCPWithPool(q *dns.Msg, pool *cpool.Pool) (r *dns.Msg, err error) {
	c := pool.Get()
	start := time.Now()

exchange:
	var isNewConn bool
	if c == nil {
		c, err = u.dialTCP()
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

	pool.Put(c)
	return r, nil
}

func (u *fastUpstream) exchangeViaTCPConn(q *dns.Msg, c net.Conn) (r *dns.Msg, err error) {
	c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err = utils.WriteMsgToTCP(c, q)
	if err != nil {
		return nil, err
	}
	c.SetReadDeadline(time.Now().Add(u.timeout))
	r, _, err = utils.ReadMsgFromTCP(c)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (u *fastUpstream) exchangeUDPWithTCPFallback(q *dns.Msg) (r *dns.Msg, err error) {
	r, err = u.exchangeUDP(q)
	if err != nil {
		return nil, err
	}

	if r != nil && r.Truncated { // fallback to tcp
		return u.exchangeTCP(q)
	}
	return r, nil
}

func (u *fastUpstream) exchangeUDP(q *dns.Msg) (r *dns.Msg, err error) {
	if u.udpPool != nil {
		return u.exchangeUDPWithPool(q, u.udpPool)
	}

	dialer := net.Dialer{Timeout: dialTimeout}
	c, err := dialer.Dial("udp", u.config.Addr)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return u.exchangeViaUDPConn(q, c, false)
}

func (u *fastUpstream) exchangeUDPWithPool(q *dns.Msg, pool *cpool.Pool) (r *dns.Msg, err error) {
	c := pool.Get()
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

	pool.Put(c)
	return r, nil
}

func (u *fastUpstream) exchangeViaUDPConn(q *dns.Msg, c net.Conn, isNewConn bool) (r *dns.Msg, err error) {
	c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err = utils.WriteMsgToUDP(c, q)
	if err != nil { // write err typically is a fatal err
		return nil, err
	}
	c.SetReadDeadline(time.Now().Add(u.timeout))

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
