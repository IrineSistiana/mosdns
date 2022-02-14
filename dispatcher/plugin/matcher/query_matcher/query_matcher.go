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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/elem"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/msg_matcher"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const PluginType = "query_matcher"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(preset(handler.NewBP("_qtype_AAAA", PluginType), &Args{QType: []int{int(dns.TypeAAAA)}}))
	handler.MustRegPlugin(preset(handler.NewBP("_qtype_A_AAAA", PluginType), &Args{QType: []int{int(dns.TypeA), int(dns.TypeAAAA)}}))
	handler.MustRegPlugin(preset(handler.NewBP("_query_is_common", PluginType), &Args{
		QType:        []int{int(dns.TypeA), int(dns.TypeAAAA)},
		QClass:       []int{dns.ClassINET},
		IsLogicalAND: true,
	}))

	handler.MustRegPlugin(&queryIsEDNS0{BP: handler.NewBP("_query_edns0", PluginType)})
}

var _ handler.MatcherPlugin = (*queryMatcher)(nil)

type Args struct {
	ClientIP     []string `yaml:"client_ip"`
	ECS          []string `yaml:"ecs"`
	Domain       []string `yaml:"domain"`
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
		ipMatcher := netlist.NewList()
		err := netlist.BatchLoad(ipMatcher, args.ClientIP)
		if err != nil {
			return nil, err
		}
		ipMatcher.Sort()
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewClientIPMatcher(ipMatcher))
		bp.L().Info("client ip matcher loaded", zap.Int("length", ipMatcher.Len()))
	}
	if len(args.ECS) > 0 {
		ipMatcher := netlist.NewList()
		err := netlist.BatchLoad(ipMatcher, args.ECS)
		if err != nil {
			return nil, err
		}
		ipMatcher.Sort()
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewClientECSMatcher(ipMatcher))
		bp.L().Info("ecs ip matcher loaded", zap.Int("length", ipMatcher.Len()))
	}
	if len(args.Domain) > 0 {
		mixMatcher := domain.NewMixMatcher(domain.WithDomainMatcher(domain.NewSimpleDomainMatcher()))
		err := domain.BatchLoadMatcher(mixMatcher, args.Domain, nil)
		if err != nil {
			return nil, err
		}
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewQNameMatcher(mixMatcher))
		bp.L().Info("domain matcher loaded", zap.Int("length", mixMatcher.Len()))
	}
	if len(args.QType) > 0 {
		elemMatcher := elem.NewIntMatcher(args.QType)
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewQTypeMatcher(elemMatcher))

	}

	if len(args.QClass) > 0 {
		elemMatcher := elem.NewIntMatcher(args.QClass)
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewQClassMatcher(elemMatcher))
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

var _ handler.MatcherPlugin = (*queryMatcher)(nil)

type queryIsEDNS0 struct {
	*handler.BP
}

func (q *queryIsEDNS0) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	return qCtx.Q().IsEdns0() != nil, nil
}
