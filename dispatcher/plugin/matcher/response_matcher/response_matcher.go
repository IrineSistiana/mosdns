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

package responsematcher

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

const PluginType = "response_matcher"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(preset(handler.NewBP("_response_rcode_success", PluginType), &Args{Rcode: []int{dns.RcodeSuccess}}))
	handler.MustRegPlugin(&emptyAnswer{handler.NewBP("_response_empty_answer", PluginType)})
}

var _ handler.MatcherPlugin = (*responseMatcher)(nil)

type Args struct {
	Rcode        []int    `yaml:"rcode"`
	IP           []string `yaml:"ip"`    // ip files
	CNAME        []string `yaml:"cname"` // domain files
	IsLogicalAND bool     `yaml:"logical_and"`
}

type responseMatcher struct {
	*handler.BP
	args *Args

	matcherGroup []handler.Matcher
}

func (m *responseMatcher) Match(ctx context.Context, qCtx *handler.Context) (matched bool, err error) {
	return utils.BoolLogic(ctx, qCtx, m.matcherGroup, m.args.IsLogicalAND)
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newResponseMatcher(bp, args.(*Args))
}

func newResponseMatcher(bp *handler.BP, args *Args) (m *responseMatcher, err error) {
	m = new(responseMatcher)
	m.BP = bp
	m.args = args

	if len(args.Rcode) > 0 {
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewRCodeMatcher(elem.NewIntMatcher(args.Rcode)))
	}

	if len(args.CNAME) > 0 {
		mixMatcher := domain.NewMixMatcher[struct{}]()
		mixMatcher.SetDefaultMatcher(domain.MatcherFull)
		err := domain.BatchLoad[struct{}](mixMatcher, args.CNAME, nil)
		if err != nil {
			return nil, err
		}
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewCNameMatcher(mixMatcher))
		bp.L().Info("cname matcher loaded", zap.Int("length", mixMatcher.Len()))
	}

	if len(args.IP) > 0 {
		ipMatcher := netlist.NewList()
		err := netlist.BatchLoad(ipMatcher, args.IP)
		if err != nil {
			return nil, err
		}
		ipMatcher.Sort()
		m.matcherGroup = append(m.matcherGroup, msg_matcher.NewAAAAAIPMatcher(ipMatcher))
		bp.L().Info("ip matcher loaded", zap.Int("length", ipMatcher.Len()))
	}

	return m, nil
}

func preset(bp *handler.BP, args *Args) (m *responseMatcher) {
	m, err := newResponseMatcher(bp, args)
	if err != nil {
		panic(fmt.Sprintf("response_matcher: failed to init pre-set plugin %s: %s", bp.Tag(), err))
	}
	return m
}
