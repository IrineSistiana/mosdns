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
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"net"
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

	ipv4 net.IP
	ipv6 net.IP
}

type Args struct {
	IPv4  string `yaml:"ipv4"` // block by responding specific IP
	IPv6  string `yaml:"ipv6"`
	RCode int    `yaml:"rcode"` // block by responding specific RCode
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newBlackHole(bp, args.(*Args))
}

func newBlackHole(bp *coremain.BP, args *Args) (*blackHole, error) {
	b := &blackHole{BP: bp, args: args}
	if len(args.IPv4) != 0 {
		ip := net.ParseIP(args.IPv4)
		if ip == nil {
			return nil, fmt.Errorf("%s is an invalid ipv4 addr", args.IPv4)
		}
		b.ipv4 = ip
	}
	if len(args.IPv6) != 0 {
		ip := net.ParseIP(args.IPv6)
		if ip == nil {
			return nil, fmt.Errorf("%s is an invalid ipv6 addr", args.IPv6)
		}
		b.ipv6 = ip
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
	case b.ipv4 != nil && qtype == dns.TypeA:
		r := new(dns.Msg)
		r.SetRcode(q, dns.RcodeSuccess)
		r.RecursionAvailable = true
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   qName,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			A: b.ipv4,
		}
		r.Answer = []dns.RR{rr}
		qCtx.SetResponse(r)

	case b.ipv6 != nil && qtype == dns.TypeAAAA:
		r := new(dns.Msg)
		r.SetRcode(q, dns.RcodeSuccess)
		r.RecursionAvailable = true
		rr := &dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   qName,
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			AAAA: b.ipv6,
		}
		r.Answer = []dns.RR{rr}
		qCtx.SetResponse(r)

	case b.args.RCode >= 0:
		r := dnsutils.GenEmptyReply(q, b.args.RCode)
		qCtx.SetResponse(r)
	default:
		qCtx.SetResponse(nil)
	}

	return
}
