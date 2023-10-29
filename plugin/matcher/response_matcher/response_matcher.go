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

package responsematcher

import (
	"context"
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/matcher/domain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/matcher/elem"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/matcher/msg_matcher"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/matcher/netlist"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
)

const PluginType = "response_matcher"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })

	coremain.RegNewPersetPluginFunc("_response_valid_answer", func(bp *coremain.BP) (coremain.Plugin, error) {
		return &hasValidAnswer{BP: bp}, nil
	})
}

var _ coremain.MatcherPlugin = (*responseMatcher)(nil)

type Args struct {
	RCode []int    `yaml:"rcode"`
	IP    []string `yaml:"ip"`
	CNAME []string `yaml:"cname"`
}

type responseMatcher struct {
	*coremain.BP
	args *Args

	matcherGroup []executable_seq.Matcher
	closer       []io.Closer
}

func (m *responseMatcher) Match(ctx context.Context, qCtx *query_context.Context) (matched bool, err error) {
	return executable_seq.LogicalAndMatcherGroup(ctx, qCtx, m.matcherGroup)
}

func (m *responseMatcher) Close() error {
	for _, closer := range m.closer {
		_ = closer.Close()
	}
	return nil
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newResponseMatcher(bp, args.(*Args))
}

func newResponseMatcher(bp *coremain.BP, args *Args) (m *responseMatcher, err error) {
	m = new(responseMatcher)
	m.BP = bp
	m.args = args

	if len(args.RCode) > 0 {
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewRCodeMatcher(elem.NewIntMatcher(args.RCode)))
	}

	if len(args.CNAME) > 0 {
		mg, err := domain.BatchLoadDomainProvider(
			args.CNAME,
			bp.M().GetDataManager(),
		)
		if err != nil {
			return nil, err
		}
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewCNameMatcher(mg))
		m.closer = append(m.closer, mg)
		bp.L().Info("cname matcher loaded", zap.Int("length", mg.Len()))
	}

	if len(args.IP) > 0 {
		l, err := netlist.BatchLoadProvider(args.IP, bp.M().GetDataManager())
		if err != nil {
			return nil, err
		}
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewAAAAAIPMatcher(l))
		m.closer = append(m.closer, l)
		bp.L().Info("ip matcher loaded", zap.Int("length", l.Len()))
	}

	return m, nil
}

type hasValidAnswer struct {
	*coremain.BP
}

var _ coremain.MatcherPlugin = (*hasValidAnswer)(nil)

func (e *hasValidAnswer) match(qCtx *query_context.Context) (matched bool) {
	r := qCtx.R()
	if r == nil {
		return false
	}

	q := qCtx.Q()
	m := make(map[dns.Question]struct{})
	for _, question := range q.Question {
		m[question] = struct{}{}
	}

	for _, rr := range r.Answer {
		h := rr.Header()
		q := dns.Question{
			Name:   h.Name,
			Qtype:  h.Rrtype,
			Qclass: h.Class,
		}
		if _, ok := m[q]; ok {
			return true
		}
	}

	return false
}

func (e *hasValidAnswer) Match(_ context.Context, qCtx *query_context.Context) (matched bool, err error) {
	return e.match(qCtx), nil
}
