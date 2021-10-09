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

package blackhole

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/miekg/dns"
	"net"
)

const PluginType = "blackhole"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(preset(handler.NewBP("_drop_response", PluginType), &Args{RCode: -1}), true)
	handler.MustRegPlugin(preset(handler.NewBP("_block_with_empty_response", PluginType), &Args{RCode: dns.RcodeSuccess}), true)
	handler.MustRegPlugin(preset(handler.NewBP("_block_with_servfail", PluginType), &Args{RCode: dns.RcodeServerFailure}), true)
	handler.MustRegPlugin(preset(handler.NewBP("_block_with_nxdomain", PluginType), &Args{RCode: dns.RcodeNameError}), true)
}

var _ handler.ExecutablePlugin = (*blackhole)(nil)

type blackhole struct {
	*handler.BP
	args *Args

	ipv4 net.IP
	ipv6 net.IP
}

type Args struct {
	IPv4  string `yaml:"ipv4"` // block by responding specific IP
	IPv6  string `yaml:"ipv6"`
	RCode int    `yaml:"rcode"` // block by responding specific RCode
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newBlackhole(bp, args.(*Args))
}

func newBlackhole(bp *handler.BP, args *Args) (*blackhole, error) {
	b := &blackhole{BP: bp, args: args}
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
func (b *blackhole) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	b.exec(qCtx)
	return handler.ExecChainNode(ctx, qCtx, next)
}

func (b *blackhole) exec(qCtx *handler.Context) {
	q := qCtx.Q()
	if len(q.Question) != 1 {
		return
	}

	qName := q.Question[0].Name
	qtype := q.Question[0].Qtype

	switch {
	case b.ipv4 != nil && qtype == dns.TypeA:
		r := new(dns.Msg)
		r.SetReply(qCtx.Q())
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   qName,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			A: b.ipv4,
		}
		r.Answer = append(r.Answer, rr)
		qCtx.SetResponse(r, handler.ContextStatusRejected)

	case b.ipv6 != nil && qtype == dns.TypeAAAA:
		r := new(dns.Msg)
		r.SetReply(qCtx.Q())
		rr := &dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   qName,
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			AAAA: b.ipv6,
		}
		r.Answer = append(r.Answer, rr)
		qCtx.SetResponse(r, handler.ContextStatusRejected)

	case b.args.RCode >= 0:
		r := new(dns.Msg)
		r.SetReply(qCtx.Q())
		r.Rcode = b.args.RCode
		qCtx.SetResponse(r, handler.ContextStatusRejected)

	default:
		qCtx.SetResponse(nil, handler.ContextStatusDropped)
	}

	return
}

func preset(bp *handler.BP, args *Args) *blackhole {
	b, err := newBlackhole(bp, args)
	if err != nil {
		panic(fmt.Sprintf("blackhole: %v", err))
	}
	return b
}
