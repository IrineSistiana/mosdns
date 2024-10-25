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

package querymatcher

import (
	"context"
	"io"
	"sync"

	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/elem"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/msg_matcher"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const PluginType = "query_matcher"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })

	coremain.RegNewPersetPluginFunc(
		"_qtype_A_AAAA",
		func(bp *coremain.BP) (coremain.Plugin, error) {
			return newQueryMatcher(bp, &Args{QType: []int{int(dns.TypeA), int(dns.TypeAAAA)}})
		},
	)
	coremain.RegNewPersetPluginFunc(
		"_qtype_AAAA",
		func(bp *coremain.BP) (coremain.Plugin, error) {
			return newQueryMatcher(bp, &Args{QType: []int{int(dns.TypeAAAA)}})
		},
	)

	coremain.RegNewPersetPluginFunc(
		"_query_edns0",
		func(bp *coremain.BP) (coremain.Plugin, error) {
			return &queryIsEDNS0{BP: bp}, nil
		},
	)
}

var _ coremain.MatcherPlugin = (*queryMatcher)(nil)

type Args struct {
	ClientIP []string `yaml:"client_ip"`
	ECS      []string `yaml:"ecs"`
	Domain   []string `yaml:"domain"`
	QType    []int    `yaml:"qtype"`
	QClass   []int    `yaml:"qclass"`
	// TODO: Add PTR matcher.
}

type queryMatcher struct {
	*coremain.BP
	args *Args

	matcherGroup []executable_seq.Matcher
	closer       []io.Closer
	mu           sync.Mutex
}

func (m *queryMatcher) Match(ctx context.Context, qCtx *query_context.Context) (matched bool, err error) {
	return executable_seq.LogicalAndMatcherGroup(ctx, qCtx, m.matcherGroup)
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newQueryMatcher(bp, args.(*Args))
}

func newQueryMatcher(bp *coremain.BP, args *Args) (m *queryMatcher, err error) {
	m = &queryMatcher{
		BP:  bp,
		args: args,
	}
	m.initMatchers()
	return m, nil
}

func (m *queryMatcher) initMatchers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.matcherGroup == nil {
		m.matcherGroup = make([]executable_seq.Matcher, 0)
	}
	if m.closer == nil {
		m.closer = make([]io.Closer, 0)
	}

	if len(m.args.ClientIP) > 0 {
		l, err := netlist.BatchLoadProvider(m.args.ClientIP, m.BP.M().GetDataManager())
		if err!= nil {
			m.BP.L().Error("failed to load client ip matcher", zap.Error(err))
			return
		}
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewClientIPMatcher(l))
		m.closer = append(m.closer, l)
		m.BP.L().Info("client ip matcher loaded", zap.Int("length", l.Len()))
	}
	if len(m.args.ECS) > 0 {
		l, err := netlist.BatchLoadProvider(m.args.ECS, m.BP.M().GetDataManager())
		if err!= nil {
			m.BP.L().Error("failed to load ecs ip matcher", zap.Error(err))
			return
		}
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewClientECSMatcher(l))
		m.closer = append(m.closer, l)
		m.BP.L().Info("ecs ip matcher loaded", zap.Int("length", l.Len()))
	}
	if len(m.args.Domain) > 0 {
		mg, err := domain.BatchLoadDomainProvider(
			m.args.Domain,
			m.BP.M().GetDataManager(),
		)
		if err!= nil {
			m.BP.L().Error("failed to load domain matcher", zap.Error(err))
			return
		}
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewQNameMatcher(mg))
		m.closer = append(m.closer, mg)
		m.BP.L().Info("domain matcher loaded", zap.Int("length", mg.Len()))
	}
	if len(m.args.QType) > 0 {
		elemMatcher := elem.NewIntMatcher(m.args.QType)
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewQTypeMatcher(elemMatcher))
	}
	if len(m.args.QClass) > 0 {
		elemMatcher := elem.NewIntMatcher(m.args.QClass)
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewQClassMatcher(elemMatcher))
	}
}

var _ coremain.MatcherPlugin = (*queryMatcher)(nil)

type queryIsEDNS0 struct {
	*coremain.BP
}

func (q *queryIsEDNS0) Match(_ context.Context, qCtx *query_context.Context) (matched bool, err error) {
	return qCtx.Q().IsEdns0()!= nil, nil
}
