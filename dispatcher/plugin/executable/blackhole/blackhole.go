//     Copyright (C) 2020, IrineSistiana
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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"net"
)

const PluginType = "blackhole"

func init() {
	handler.RegInitFunc(PluginType, Init)

	handler.MustRegPlugin(handler.WrapExecutablePlugin("_drop_response", PluginType, &blackhole{args: &Args{RCode: -1}}))
	handler.MustRegPlugin(handler.WrapExecutablePlugin("_block_with_servfail", PluginType, &blackhole{args: &Args{RCode: dns.RcodeServerFailure}}))
	handler.MustRegPlugin(handler.WrapExecutablePlugin("_block_with_nxdomain", PluginType, &blackhole{args: &Args{RCode: dns.RcodeNameError}}))
}

var _ handler.Executable = (*blackhole)(nil)

type blackhole struct {
	args *Args
	ipv4 net.IP
	ipv6 net.IP
}

type Args struct {
	IPv4  string `yaml:"ipv4"` // block by responding specific IP
	IPv6  string `yaml:"ipv6"`
	RCode int    `yaml:"rcode"` // block by responding specific RCode
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	b, err := newBlackhole(tag, args)
	if err != nil {
		return nil, err
	}
	return handler.WrapExecutablePlugin(tag, PluginType, b), nil
}

func newBlackhole(tag string, args *Args) (*blackhole, error) {
	b := &blackhole{args: args}
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
// sets qCtx.R with IP response if query type is A/AAAA and Args.IPv4 / Args.IPv6 is not empty.
// sets qCtx.R with empty response with rcode = Args.RCode.
// drops qCtx.R if Args.RCode < 0
// It never returns an err.
func (b *blackhole) Exec(_ context.Context, qCtx *handler.Context) (err error) {
	switch {
	case b.ipv4 != nil && len(qCtx.Q.Question) == 1 && qCtx.Q.Question[0].Qtype == dns.TypeA:
		r := new(dns.Msg)
		r.SetReply(qCtx.Q)
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   qCtx.Q.Question[0].Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			A: b.ipv4,
		}
		r.Answer = append(r.Answer, rr)
		qCtx.SetResponse(r, handler.ContextStatusRejected)

	case b.ipv6 != nil && len(qCtx.Q.Question) == 1 && qCtx.Q.Question[0].Qtype == dns.TypeAAAA:
		r := new(dns.Msg)
		r.SetReply(qCtx.Q)
		rr := &dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   qCtx.Q.Question[0].Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			AAAA: b.ipv6,
		}
		r.Answer = append(r.Answer, rr)
		qCtx.SetResponse(r, handler.ContextStatusRejected)

	case b.args.RCode >= 0:
		r := new(dns.Msg)
		r.SetReply(qCtx.Q)
		r.Rcode = b.args.RCode
		qCtx.SetResponse(r, handler.ContextStatusRejected)

	default:
		qCtx.SetResponse(nil, handler.ContextStatusDropped)
	}

	return nil
}
