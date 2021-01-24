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

package querymatcher

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/domain"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/elem"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/netlist"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
)

const PluginType = "query_matcher"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(preset(handler.NewBP("_qtype_AAAA", PluginType), &Args{QType: []int{int(dns.TypeAAAA)}}), true)
	handler.MustRegPlugin(preset(handler.NewBP("_qtype_A_AAAA", PluginType), &Args{QType: []int{int(dns.TypeA), int(dns.TypeAAAA)}}), true)
	handler.MustRegPlugin(preset(handler.NewBP("_query_is_common", PluginType), &Args{
		QType:        []int{int(dns.TypeA), int(dns.TypeAAAA)},
		QClass:       []int{dns.ClassINET},
		IsLogicalAND: true,
	}), true)
}

var _ handler.MatcherPlugin = (*queryMatcher)(nil)

type Args struct {
	ClientIP     []string `yaml:"client_ip"` // ip files
	Domain       []string `yaml:"domain"`    // domain files
	QType        []int    `yaml:"qtype"`
	QClass       []int    `yaml:"qclass"`
	IsLogicalAND bool     `yaml:"logical_and"`
}

type queryMatcher struct {
	*handler.BP
	args *Args

	matcherGroup []handler.Matcher
}

func (m *queryMatcher) Match(ctx context.Context, qCtx *handler.Context) (matched bool, err error) {
	return utils.BoolLogic(ctx, qCtx, m.matcherGroup, m.args.IsLogicalAND)
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newQueryMatcher(bp, args.(*Args))
}

func newQueryMatcher(bp *handler.BP, args *Args) (m *queryMatcher, err error) {
	m = new(queryMatcher)
	m.BP = bp
	m.args = args

	if len(args.ClientIP) > 0 {
		ipMatcher, err := netlist.BatchLoad(args.ClientIP)
		if err != nil {
			return nil, err
		}
		m.matcherGroup = append(m.matcherGroup, newClientIPMatcher(ipMatcher))
	}
	if len(args.Domain) > 0 {
		mixMatcher := domain.NewMixMatcher()
		err := domain.BatchLoadMixMatcherV2Matcher(mixMatcher, args.Domain)
		if err != nil {
			return nil, err
		}
		m.matcherGroup = append(m.matcherGroup, newQDomainMatcher(mixMatcher))
	}
	if len(args.QType) > 0 {
		elemMatcher := elem.NewIntMatcher(args.QType)
		m.matcherGroup = append(m.matcherGroup, newQTypeMatcher(elemMatcher))
	}

	if len(args.QClass) > 0 {
		elemMatcher := elem.NewIntMatcher(args.QClass)
		m.matcherGroup = append(m.matcherGroup, newQClassMatcher(elemMatcher))
	}

	return m, nil
}

func preset(bp *handler.BP, args *Args) (m *queryMatcher) {
	m, err := newQueryMatcher(bp, args)
	if err != nil {
		panic(fmt.Sprintf("query_matcher: failed to init pre-set plugin %s: %s", bp.Tag(), err))
	}
	return m
}
