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

package resp_ip

import (
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/IrineSistiana/mosdns/v5/plugin/matcher/base_ip"
	"github.com/miekg/dns"
	"net"
	"net/netip"
	"slices"
)

const PluginType = "svcb_iphint"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

type Args = base_ip.Args

func QuickSetup(bq sequence.BQ, s string) (sequence.Matcher, error) {
	return base_ip.NewMatcher(bq, base_ip.ParseQuickSetupArgs(s), match)
}

func matchIPHint(kvs []dns.SVCBKeyValue, m netlist.Matcher) bool {
	var hs []net.IP
	for _, kv := range kvs {
		switch kv.Key() {
		case dns.SVCB_IPV4HINT:
			hs = kv.(*dns.SVCBIPv4Hint).Hint
		case dns.SVCB_IPV6HINT:
			hs = kv.(*dns.SVCBIPv6Hint).Hint
		}
		for _, ip := range hs {
			addr, ok := netip.AddrFromSlice(ip)
			if ok && m.Match(addr) {
				return true
			}
		}
	}
	return false
}

var svcbTypes = []uint16{dns.TypeSVCB, dns.TypeHTTPS}

func match(qCtx *query_context.Context, m netlist.Matcher) (bool, error) {
	r := qCtx.R()
	if r == nil {
		return false, nil
	}

	if len(r.Question) != 1 || !slices.Contains(svcbTypes, r.Question[0].Qtype) {
		return false, nil
	}

	for _, rr := range r.Answer {
		switch rr.Header().Rrtype {
		case dns.TypeSVCB:
			return matchIPHint(rr.(*dns.SVCB).Value, m), nil
		case dns.TypeHTTPS:
			return matchIPHint(rr.(*dns.HTTPS).Value, m), nil
		}
	}

	return false, nil
}
