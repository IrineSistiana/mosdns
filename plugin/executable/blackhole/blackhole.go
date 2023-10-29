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

package blackhole

import (
	"context"
	"fmt"
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/dnsutils"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/miekg/dns"
	"net/netip"
)

const PluginType = "blackhole"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })

	coremain.RegNewPersetPluginFunc("_drop_response", func(bp *coremain.BP) (coremain.Plugin, error) {
		return newBlackHole(bp, &Args{RCode: -1})
	})
	coremain.RegNewPersetPluginFunc("_new_empty_response", func(bp *coremain.BP) (coremain.Plugin, error) {
		return newBlackHole(bp, &Args{RCode: dns.RcodeSuccess})
	})
	coremain.RegNewPersetPluginFunc("_new_servfail_response", func(bp *coremain.BP) (coremain.Plugin, error) {
		return newBlackHole(bp, &Args{RCode: dns.RcodeServerFailure})
	})
	coremain.RegNewPersetPluginFunc("_new_nxdomain_response", func(bp *coremain.BP) (coremain.Plugin, error) {
		return newBlackHole(bp, &Args{RCode: dns.RcodeNameError})
	})
}

var _ coremain.ExecutablePlugin = (*blackHole)(nil)

type blackHole struct {
	*coremain.BP
	args *Args

	ipv4 []netip.Addr
	ipv6 []netip.Addr
}

type Args struct {
	IPv4  []string `yaml:"ipv4"` // block by responding specific IP
	IPv6  []string `yaml:"ipv6"`
	RCode int      `yaml:"rcode"` // block by responding specific RCode
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newBlackHole(bp, args.(*Args))
}

func newBlackHole(bp *coremain.BP, args *Args) (*blackHole, error) {
	b := &blackHole{BP: bp, args: args}
	for _, s := range args.IPv4 {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid ipv4 addr %s, %w", s, err)
		}
		if !addr.Is4() {
			return nil, fmt.Errorf("invalid ipv4 addr %s", s)
		}
		b.ipv4 = append(b.ipv4, addr)
	}
	for _, s := range args.IPv6 {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid ipv6 addr %s, %w", s, err)
		}
		if !addr.Is6() {
			return nil, fmt.Errorf("invalid ipv6 addr %s", s)
		}
		b.ipv6 = append(b.ipv6, addr)
	}
	return b, nil
}

// Exec
// sets qCtx.R() with IP response if query type is A/AAAA and Args.IPv4 / Args.IPv6 is not empty.
// sets qCtx.R() with empty response with rcode = Args.RCode.
// drops qCtx.R() if Args.RCode < 0
// It never returns an error.
func (b *blackHole) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	b.exec(qCtx)
	return executable_seq.ExecChainNode(ctx, qCtx, next)
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
		r.RecursionAvailable = true
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
		r.RecursionAvailable = true
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

	case b.args.RCode >= 0:
		r := dnsutils.GenEmptyReply(q, b.args.RCode)
		qCtx.SetResponse(r)
	default:
		qCtx.SetResponse(nil)
	}

	return
}
