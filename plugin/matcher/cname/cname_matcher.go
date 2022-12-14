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

package qname_matcher

import (
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/IrineSistiana/mosdns/v5/plugin/matcher/base_domain"
	"github.com/miekg/dns"
)

const PluginType = "cname"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

type Args = base_domain.Args

func QuickSetup(bq sequence.BQ, s string) (sequence.Matcher, error) {
	return base_domain.NewMatcher(bq, base_domain.ParseQuickSetupArgs(s), matchCName)
}

func matchCName(qCtx *query_context.Context, m domain.Matcher[struct{}]) (bool, error) {
	r := qCtx.R()
	if r == nil {
		return false, nil
	}
	for _, rr := range r.Answer {
		if cname, ok := rr.(*dns.CNAME); ok {
			if _, ok := m.Match(cname.Target); ok {
				return true, nil
			}
		}
	}
	return false, nil
}
