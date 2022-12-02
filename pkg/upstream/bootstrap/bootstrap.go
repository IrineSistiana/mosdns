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

package bootstrap

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/miekg/dns"
	"net"
	"strings"
	"sync"
	"time"
)

// NewBootstrap returns a customized *net.Resolver which can be used as a Bootstrap for a
// certain domain. Its Dial func is modified to dial s through udp.
// It also has a small built-in cache.
// s SHOULD be a literal IP address and the port SHOULD also be literal.
// Port can be omitted. In this case, the default port is :53.
// e.g. NewBootstrap("8.8.8.8"), NewBootstrap("127.0.0.1:5353").
// If s is empty, NewBootstrap returns nil. (A nil *net.Resolver is valid in net.Dialer.)
// Note that not all platform support a customized *net.Resolver. It also depends on the
// version of go runtime.
// See the package docs from the net package for more info.
func NewBootstrap(s string) *net.Resolver {
	if len(s) == 0 {
		return nil
	}
	// Add port.
	_, _, err := net.SplitHostPort(s)
	if err != nil { // no port, add it.
		s = net.JoinHostPort(strings.Trim(s, "[]"), "53")
	}

	bs := newBootstrap(s)

	return &net.Resolver{
		PreferGo:     true,
		StrictErrors: false,
		Dial:         bs.dial,
	}
}

type bootstrap struct {
	upstream string
	cache    *cache
}

func newBootstrap(upstream string) *bootstrap {
	return &bootstrap{
		upstream: upstream,
		cache:    newCache(0),
	}
}

func (b *bootstrap) dial(_ context.Context, _, _ string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	go func() {
		_ = b.handlePipe(c2)
		_ = c2.Close()
	}()
	return c1, nil
}

func (b *bootstrap) handlePipe(c net.Conn) error {
	q, _, err := dnsutils.ReadMsgFromTCP(c)
	if err != nil {
		return err
	}

	var resp *dns.Msg
	if len(q.Question) == 1 {
		k := q.Question[0]
		m := b.cache.lookup(k)
		if m != nil {
			resp = m.Copy()
			resp.Id = q.Id
		}
	}

	if resp == nil {
		d := net.Dialer{}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()
		upstreamConn, err := d.DialContext(ctx, "udp", b.upstream)
		if err != nil {
			return err
		}
		defer upstreamConn.Close()
		_ = upstreamConn.SetDeadline(time.Now().Add(time.Second * 3))
		if _, err := dnsutils.WriteMsgToUDP(upstreamConn, q); err != nil {
			return err
		}
		m, _, err := dnsutils.ReadMsgFromUDP(upstreamConn, 1500)
		if err != nil {
			return err
		}
		resp = m
	}

	if resp.Rcode == dns.RcodeSuccess && hasIP(resp) {
		if len(resp.Question) == 1 {
			k := resp.Question[0]
			ttl := time.Duration(dnsutils.GetMinimalTTL(resp)) * time.Second
			b.cache.store(k, resp, ttl)
		}
	}

	_, err = dnsutils.WriteMsgToTCP(c, resp)
	return err
}

func hasIP(m *dns.Msg) bool {
	for _, rr := range m.Answer {
		switch rr.Header().Rrtype {
		case dns.TypeA, dns.TypeAAAA:
			return true
		}
	}
	return false
}

type cache struct {
	l int

	m sync.Mutex
	c map[key]*elem
}

type elem struct {
	m              *dns.Msg
	expirationTime time.Time
}

type key = dns.Question

func newCache(size int) *cache {
	const defaultSize = 8
	if size <= 0 {
		size = defaultSize
	}
	return &cache{
		l: size,
		c: make(map[key]*elem),
	}
}

// lookup returns a cached msg. Note: the msg must not be modified.
func (c *cache) lookup(k key) *dns.Msg {
	now := time.Now()
	c.m.Lock()
	defer c.m.Unlock()
	e, ok := c.c[k]
	if !ok {
		return nil
	}
	if e.expirationTime.Before(now) {
		delete(c.c, k)
		return nil
	}
	return e.m
}

// store stores a msg to cache. The caller MUST NOT modify m anymore.
func (c *cache) store(k key, m *dns.Msg, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	expirationTime := time.Now().Add(ttl)

	c.m.Lock()
	defer c.m.Unlock()

	if len(c.c)+1 > c.l {
		for k := range c.c {
			if len(c.c)+1 <= c.l {
				break
			}
			delete(c.c, k)
		}
	}

	c.c[k] = &elem{
		m:              m,
		expirationTime: expirationTime,
	}
}
