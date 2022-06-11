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

package fastforward

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/pkg/dnsutils"
	"net"
	"time"

	"github.com/miekg/dns"
)

type udpmeUpstream struct {
	addr    string
	trusted bool
}

func newUDPME(addr string, trusted bool) *udpmeUpstream {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "53")
	}
	return &udpmeUpstream{addr: addr, trusted: trusted}
}

func (u *udpmeUpstream) Address() string {
	return u.addr
}

func (u *udpmeUpstream) Trusted() bool {
	return u.trusted
}

func (u *udpmeUpstream) Exchange(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	ddl, ok := ctx.Deadline()
	if !ok {
		ddl = time.Now().Add(time.Second * 3)
	}

	if m.IsEdns0() != nil {
		return u.exchangeOPTM(m, ddl)
	}
	mc := m.Copy()
	mc.SetEdns0(512, false)
	r, err := u.exchangeOPTM(mc, ddl)
	if err != nil {
		return nil, err
	}
	dnsutils.RemoveEDNS0(r)
	return r, nil
}

func (u *udpmeUpstream) exchangeOPTM(m *dns.Msg, ddl time.Time) (*dns.Msg, error) {
	c, err := dns.Dial("udp", u.addr)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(ddl)
	if opt := m.IsEdns0(); opt != nil {
		c.UDPSize = opt.UDPSize()
	}

	if err := c.WriteMsg(m); err != nil {
		return nil, err
	}

	for {
		r, err := c.ReadMsg()
		if err != nil {
			return nil, err
		}
		if r.IsEdns0() == nil {
			continue
		}
		return r, nil
	}
}
