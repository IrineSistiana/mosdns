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

	handler.MustRegPlugin(preset("_qtype_A_AAAA", &Args{QType: []int{int(dns.TypeA), int(dns.TypeAAAA)}}))
	handler.MustRegPlugin(preset("_query_is_common", &Args{QType: []int{int(dns.TypeA), int(dns.TypeAAAA)},
		QClass: []int{dns.ClassINET}}))

}

var _ handler.Matcher = (*queryMatcher)(nil)

type Args struct {
	ClientIP []string `yaml:"client_ip"` // ip files
	Domain   []string `yaml:"domain"`    // domain files
	QType    []int    `yaml:"qtype"`
	QClass   []int    `yaml:"qclass"`
}

type queryMatcher struct {
	tag    string
	logger *logrus.Entry

	clientIP netlist.Matcher
	domain   domain.Matcher
	qClass   *elem.IntMatcher
	qType    *elem.IntMatcher
}

func (m *queryMatcher) Tag() string {
	return m.tag
}

func (m *queryMatcher) Type() string {
	return PluginType
}

func (m *queryMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	for i := range qCtx.Q.Question {
		if m.clientIP != nil && qCtx.From != nil {
			ip := utils.GetIPFromAddr(qCtx.From)
			if ip != nil {
				if m.clientIP.Match(ip) {
					return true, nil
				}
			} else {
				m.logger.Warnf("internal err: client addr [%s] is invalid", qCtx.From)
			}
		}

		if m.domain != nil {
			_, ok := m.domain.Match(qCtx.Q.Question[i].Name)
			if ok {
				return true, nil
			}
		}

		if m.qType != nil {
			if m.qType.Match(int(qCtx.Q.Question[i].Qtype)) {
				return true, nil
			}
		}

		if m.qClass != nil {
			if m.qClass.Match(int(qCtx.Q.Question[i].Qclass)) {
				return true, nil
			}
		}
	}

	return false, nil
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

	if len(args.ClientIP) > 0 {
		m.clientIP, err = netlist.BatchLoad(args.ClientIP)
		if err != nil {
			return nil, err
		}
	}
	if len(args.Domain) > 0 {
		mixMatcher, err := domain.BatchLoadMixMatcherV2Matcher(args.Domain)
		if err != nil {
			return nil, err
		}
		m.domain = mixMatcher
	}
	if len(args.QType) > 0 {
		m.qType = elem.NewIntMatcher(args.QType)
	}

	if len(args.QClass) > 0 {
		m.qClass = elem.NewIntMatcher(args.QClass)
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
