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

package resp_ip_mmdb

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"github.com/oschwald/geoip2-golang"
	"net"
	"strings"
)

const PluginType = "resp_ip_mmdb"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

func QuickSetup(bq sequence.BQ, s string) (sequence.Matcher, error) {
	if len(s) == 0 {
		return nil, errors.New("a iso code probability is required")
	}

	args := strings.Fields(s)
	if len(args) != 2 {
		return nil, errors.New("probability error, must like: resp_ip_mmdb $plugin_name CN")
	}

	mmdbName, _ := cutPrefix(args[0], "$")

	p := bq.M().GetPlugin(mmdbName)
	provider, _ := p.(data_provider.MmdbMatcherProvider)
	if provider == nil {
		return nil, fmt.Errorf("cannot find mmdb %s", mmdbName)
	}
	m := provider.GetMmdbMatcher()

	return &Matcher{args[1], m}, nil
}

type Matcher struct {
	isoCode string
	mmdb    *geoip2.Reader
}

func (m *Matcher) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	r := qCtx.R()
	if r == nil {
		return false, nil
	}

	if m.mmdb == nil {
		return false, nil
	}

	for _, rr := range r.Answer {
		var ip net.IP
		switch rr := rr.(type) {
		case *dns.A:
			ip = rr.A
		case *dns.AAAA:
			ip = rr.AAAA
		default:
			continue
		}

		record, err := m.mmdb.Country(ip)
		if err != nil {
			continue
		}

		if record.Country.IsoCode == m.isoCode {
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
