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
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/miekg/dns"
	"golang.org/x/net/proxy"
	"net"
)

type socketOpts struct {
	so_mark        int
	bind_to_device string
}

func dialTCP(ctx context.Context, addr, socks5 string, dialer *net.Dialer) (net.Conn, error) {
	if len(socks5) > 0 {
		socks5Dialer, err := proxy.SOCKS5("tcp", socks5, nil, dialer)
		if err != nil {
			return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
		}
		return socks5Dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", addr)
	}

	return dialer.DialContext(ctx, "tcp", addr)
}

// GoResolverDialerWrapper wraps an Upstream to a dialer that
// can be used in net.Resolver.
type GoResolverDialerWrapper struct {
	u Upstream
}

func NewGoResolverDialer(u Upstream) *GoResolverDialerWrapper {
	return &GoResolverDialerWrapper{u: u}
}

func (d *GoResolverDialerWrapper) Dial(_ context.Context, _ string, _ string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	go d.servePipe(c2)
	return c1, nil
}

func (d *GoResolverDialerWrapper) servePipe(c net.Conn) {
	defer c.Close()

	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		m, _, err := dnsutils.ReadMsgFromTCP(c)
		if err != nil {
			return
		}

		go func() {
			r, _ := d.u.ExchangeContext(connCtx, m)
			if r == nil {
				r = new(dns.Msg)
				r.SetRcode(m, dns.RcodeServerFailure)
			}
			_, _ = dnsutils.WriteMsgToTCP(c, r)
		}()
	}
}
