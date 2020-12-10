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

package ipmatcher

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/netlist"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
)

const PluginType = "ip_matcher"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

var _ handler.Matcher = (*ipMatcher)(nil)

type Args struct {
	MatchResponse bool     `yaml:"match_response"`
	MatchClient   bool     `yaml:"match_client"`
	IP            []string `yaml:"ip"`
}

type ipMatcher struct {
	args         *Args
	matcherGroup netlist.Matcher
	logger       *logrus.Entry
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.IP) == 0 {
		return nil, errors.New("no ip file is configured")
	}

	m := new(ipMatcher)
	m.args = args
	m.logger = mlog.NewPluginLogger(tag)
	mg := make([]netlist.Matcher, 0, len(args.IP))
	for _, f := range args.IP {
		matcher, err := netlist.NewIPMatcherFromFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to load ip file %s: %w", f, err)
		}
		mg = append(mg, matcher)
	}

	m.matcherGroup = netlist.NewMatcherGroup(mg)

	return handler.WrapMatcherPlugin(tag, PluginType, m), nil
}

func (m *ipMatcher) Match(_ context.Context, qCtx *handler.Context) (bool, error) {
	if qCtx == nil {
		return false, nil
	}

	if m.args.MatchResponse && qCtx.R != nil {
		ok := m.matchResponse(qCtx.R.Answer)
		if ok {
			return true, nil
		}
	}

	if m.args.MatchClient && qCtx.From != nil {
		ip := utils.GetIPFromAddr(qCtx.From)
		if ip != nil && m.matcherGroup.Match(ip) {
			return true, nil
		}
	}

	return false, nil
}

func (m *ipMatcher) matchResponse(a []dns.RR) bool {
	for i := range a {
		var ip net.IP
		switch rr := a[i].(type) {
		case *dns.A:
			ip = rr.A
		case *dns.AAAA:
			ip = rr.AAAA
		default:
			continue
		}

		if m.matcherGroup.Match(ip) {
			return true
		}
	}
	return false
}
