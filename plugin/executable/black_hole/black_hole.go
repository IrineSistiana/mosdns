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

package black_hole

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"net/netip"
	"strings"
)

const PluginType = "black_hole"

func init() {
	sequence.MustRegQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*blackHole)(nil)

type blackHole struct {
	ipv4 []netip.Addr
	ipv6 []netip.Addr
}

// QuickSetup format: [ipv4|ipv6] ...
// Support both ipv4/a and ipv6/aaaa families. If one of family is not set, an
// unspecified ip (0.0.0.0 and ::) will be used.
func QuickSetup(_ sequence.BQ, s string) (any, error) {
	return newBlackHole(strings.Fields(s))
}

func newBlackHole(args []string) (*blackHole, error) {
	b := &blackHole{}
	for _, s := range args {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid ipv4 addr %s, %w", s, err)
		}
		if addr.Is4() {
			b.ipv4 = append(b.ipv4, addr)
		} else {
			b.ipv6 = append(b.ipv6, addr)
		}
	}

	if len(b.ipv4) == 0 {
		b.ipv4 = append(b.ipv4, netip.IPv4Unspecified())
	}
	if len(b.ipv6) == 0 {
		b.ipv6 = append(b.ipv6, netip.IPv6Unspecified())
	}
	return b, nil
}

func (b *blackHole) Exec(_ context.Context, qCtx *query_context.Context) error {
	b.exec(qCtx)
	return nil
}

func (b *blackHole) exec(qCtx *query_context.Context) {
	q := qCtx.Q()
	if len(q.Question) != 1 {
		return
	}

	qName := q.Question[0].Name
	qtype := q.Question[0].Qtype

	switch {
	case qtype == dns.TypeA && len(b.ipv4) > 0:
		r := new(dns.Msg)
		r.SetRcode(q, dns.RcodeSuccess)
		for _, addr := range b.ipv4 {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    3600,
				},
				A: addr.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
		qCtx.SetResponse(r)

	case qtype == dns.TypeAAAA && len(b.ipv6) > 0:
		r := new(dns.Msg)
		r.SetRcode(q, dns.RcodeSuccess)
		for _, addr := range b.ipv6 {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    3600,
				},
				AAAA: addr.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
		qCtx.SetResponse(r)
	}
	return
}
