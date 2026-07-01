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

package resp_ip_geoip

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const PluginType = "resp_ip_geoip"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

func QuickSetup(bq sequence.BQ, s string) (sequence.Matcher, error) {
	if len(s) == 0 {
		return nil, errors.New("a geoip plugin and country code are required")
	}

	args := strings.Fields(s)
	if len(args) != 2 {
		return nil, errors.New("args error, must like: resp_ip_geoip $plugin_name CN")
	}

	geoIPName, _ := cutPrefix(args[0], "$")

	p := bq.M().GetPlugin(geoIPName)
	provider, _ := p.(data_provider.GeoIPMatcherProvider)
	if provider == nil {
		return nil, fmt.Errorf("cannot find geoip %s", geoIPName)
	}
	m, ok := provider.GetGeoIPMatcher(args[1])
	if !ok {
		return nil, fmt.Errorf("cannot find geoip code %s", args[1])
	}

	return &Matcher{m: m}, nil
}

type Matcher struct {
	m netlist.Matcher
}

func (m *Matcher) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	r := qCtx.R()
	if r == nil {
		return false, nil
	}

	for _, rr := range r.Answer {
		var addr netip.Addr
		switch rr := rr.(type) {
		case *dns.A:
			addr, _ = netip.AddrFromSlice(rr.A)
		case *dns.AAAA:
			addr, _ = netip.AddrFromSlice(rr.AAAA)
		default:
			continue
		}
		if addr.IsValid() && m.m.Match(addr) {
			return true, nil
		}
	}

	return false, nil
}

func cutPrefix(s string, p string) (string, bool) {
	if strings.HasPrefix(s, p) {
		return strings.TrimPrefix(s, p), true
	}
	return s, false
}
