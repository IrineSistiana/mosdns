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
	"math/rand"
	"sync"
)

const PluginType = "black_hole"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*BlackHole)(nil)

type BlackHole struct {
	ipv4 []netip.Addr
	ipv6 []netip.Addr
	shuffleMutex sync.Mutex
}

// QuickSetup format: [ipv4|ipv6] ...
// Support both ipv4/a and ipv6/aaaa families.
func QuickSetup(_ sequence.BQ, s string) (any, error) {
	return NewBlackHole(strings.Fields(s))
}

// NewBlackHole creates a new BlackHole with given ips.
func NewBlackHole(ips []string) (*BlackHole, error) {
	b := &BlackHole{}
	for _, s := range ips {
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
	return b, nil
}

func (b *BlackHole) shuffleIPs() {
	b.shuffleMutex.Lock()
	defer b.shuffleMutex.Unlock()
	rand.Shuffle(len(b.ipv4), func(i, j int) {
		b.ipv4[i], b.ipv4[j] = b.ipv4[j], b.ipv4[i]
	})
	rand.Shuffle(len(b.ipv6), func(i, j int) {
		b.ipv6[i], b.ipv6[j] = b.ipv6[j], b.ipv6[i]
	})
}

// Exec implements sequence.Executable. It set a response with given ips if
// query has corresponding qtypes.
func (b *BlackHole) Exec(_ context.Context, qCtx *query_context.Context) error {
	b.shuffleIPs()
	if r := b.Response(qCtx.Q()); r != nil {
		qCtx.SetResponse(r)
	}
	return nil
}

// Response returns a response with given ips if query has corresponding qtypes.
// Otherwise, it returns nil.
func (b *BlackHole) Response(q *dns.Msg) *dns.Msg {
	if len(q.Question) != 1 {
		return nil
	}

	qName := q.Question[0].Name
	qtype := q.Question[0].Qtype

	switch {
	case qtype == dns.TypeA && len(b.ipv4) > 0:
		r := new(dns.Msg)
		r.SetReply(q)
		for _, addr := range b.ipv4 {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				A: addr.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
		return r

	case qtype == dns.TypeAAAA && len(b.ipv6) > 0:
		r := new(dns.Msg)
		r.SetReply(q)
		for _, addr := range b.ipv6 {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				AAAA: addr.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
		return r
	}
	return nil
}
