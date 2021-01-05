//     Copyright (C) 2020, IrineSistiana
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
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

const PluginType = "query_matcher"

func init() {
	handler.RegInitFunc(PluginType, Init)

	handler.MustRegPlugin(preset("_qtype_AAAA", &Args{QType: []int{int(dns.TypeAAAA)}}))
	handler.MustRegPlugin(preset("_qtype_A_AAAA", &Args{QType: []int{int(dns.TypeA), int(dns.TypeAAAA)}}))
	handler.MustRegPlugin(preset("_query_is_common", &Args{
		QType:  []int{int(dns.TypeA), int(dns.TypeAAAA)},
		QClass: []int{dns.ClassINET},
	}))

}

var _ handler.Matcher = (*queryMatcher)(nil)

type Args struct {
	ClientIP     []string `yaml:"client_ip"` // ip files
	Domain       []string `yaml:"domain"`    // domain files
	QType        []int    `yaml:"qtype"`
	QClass       []int    `yaml:"qclass"`
	IsLogicalAND bool     `yaml:"logical_and"`
}

type queryMatcher struct {
	tag    string
	logger *logrus.Entry
	args   *Args

	matcherGroup []handler.Matcher
}

func (m *queryMatcher) Tag() string {
	return m.tag
}

func (m *queryMatcher) Type() string {
	return PluginType
}

func (m *queryMatcher) Match(ctx context.Context, qCtx *handler.Context) (matched bool, err error) {
	matched, err = utils.BoolLogic(ctx, qCtx, m.matcherGroup, m.args.IsLogicalAND)
	if err != nil {
		err = handler.NewPluginError(m.tag, err)
	}
	return
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	m, err := newQueryMatcher(tag, args)
	if err != nil {
		return nil, err
	}
	return handler.WrapMatcherPlugin(tag, PluginType, m), nil
}

func newQueryMatcher(tag string, args *Args) (m *queryMatcher, err error) {
	m = new(queryMatcher)
	m.tag = tag
	m.logger = mlog.NewPluginLogger(tag)
	m.args = args

	if len(args.ClientIP) > 0 {
		ipMatcher, err := netlist.BatchLoad(args.ClientIP)
		if err != nil {
			return nil, err
		}
		m.matcherGroup = append(m.matcherGroup, newClientIPMatcher(ipMatcher))
	}
	if len(args.Domain) > 0 {
		mixMatcher, err := domain.BatchLoadMixMatcherV2Matcher(args.Domain)
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

func preset(tag string, args *Args) (m *queryMatcher) {
	m, err := newQueryMatcher(tag, args)
	if err != nil {
		panic(fmt.Sprintf("query_matcher: failed to init pre-set plugin %s: %s", tag, err))
	}
	return m
}
