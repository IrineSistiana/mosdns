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

package base_ip

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider/ip_set"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"strings"
)

var _ sequence.Matcher = (*Matcher)(nil)

type Args struct {
	IPs    []string `yaml:"ips"`
	IPSets []string `yaml:"ip_sets"`
	Files  []string `yaml:"files"`
}

type MatchFunc func(qCtx *query_context.Context, m netlist.Matcher) (bool, error)

type Matcher struct {
	match MatchFunc

	mg []netlist.Matcher
}

func (m *Matcher) Match(_ context.Context, qCtx *query_context.Context) (matched bool, err error) {
	return m.match(qCtx, ip_set.MatcherGroup(m.mg))
}

func NewMatcher(bq sequence.BQ, args *Args, f MatchFunc) (m *Matcher, err error) {
	m = &Matcher{
		match: f,
	}

	// Acquire lists from other plugins or files.
	for _, tag := range args.IPSets {
		p := bq.M().GetPlugin(tag)
		provider, _ := p.(data_provider.IPMatcherProvider)
		if provider == nil {
			return nil, fmt.Errorf("cannot find ipset %s", tag)
		}
		l := provider.GetIPMatcher()
		m.mg = append(m.mg, l)
	}

	// Anonymous set from plugin's args and files.
	if len(args.IPs)+len(args.Files) > 0 {
		anonymousList := netlist.NewList()
		if err := ip_set.LoadFromIPsAndFiles(args.IPs, args.Files, anonymousList); err != nil {
			return nil, err
		}
		anonymousList.Sort()
		if anonymousList.Len() > 0 {
			m.mg = append(m.mg, anonymousList)
		}
	}

	return m, nil
}

// ParseQuickSetupArgs parses expressions and "ip_set"s to args.
// Format: "([ip] | [$ip_set_tag] | [&ip_list_file])..."
func ParseQuickSetupArgs(s string) *Args {
	cutPrefix := func(s string, p string) (string, bool) {
		if strings.HasPrefix(s, p) {
			return strings.TrimPrefix(s, p), true
		}
		return s, false
	}

	args := new(Args)
	for _, exp := range strings.Fields(s) {
		if tag, ok := cutPrefix(exp, "$"); ok {
			args.IPSets = append(args.IPSets, tag)
		} else if path, ok := cutPrefix(exp, "&"); ok {
			args.Files = append(args.Files, path)
		} else {
			args.IPs = append(args.IPs, exp)
		}
	}
	return args
}
