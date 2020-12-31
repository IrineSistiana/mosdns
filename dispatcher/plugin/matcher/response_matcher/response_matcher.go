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

package responsematcher

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/domain"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/elem"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/netlist"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
)

const PluginType = "response_matcher"

func init() {
	handler.RegInitFunc(PluginType, Init)

	handler.MustRegPlugin(preset("_response_rcode_success", &Args{Rcode: []int{dns.RcodeSuccess}}))
}

var _ handler.Matcher = (*responseMatcher)(nil)

type Args struct {
	Rcode []int    `yaml:"rcode"`
	IP    []string `yaml:"ip"`    // ip files
	CNAME []string `yaml:"cname"` // domain files
}

type responseMatcher struct {
	tag    string
	logger *logrus.Entry
	args   *Args

	rcodeMatcher *elem.IntMatcher
	ipMatcher    netlist.Matcher
	cnameMatcher domain.Matcher
}

func (m *responseMatcher) Tag() string {
	return m.tag
}

func (m *responseMatcher) Type() string {
	return PluginType
}

func (m *responseMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	return m.match(qCtx), nil
}

func (m *responseMatcher) match(qCtx *handler.Context) (matched bool) {
	if qCtx.R == nil {
		return false
	}

	r := qCtx.R

	if m.rcodeMatcher != nil && m.rcodeMatcher.Match(r.Rcode) {
		return true
	}

	for _, rr := range r.Answer {
		if m.cnameMatcher != nil {
			if cname, ok := rr.(*dns.CNAME); ok {
				if _, ok := m.cnameMatcher.Match(cname.Target); ok {
					return true
				}
			}
		}

		if m.ipMatcher != nil {
			var ip net.IP
			switch rr := rr.(type) {
			case *dns.A:
				ip = rr.A
			case *dns.AAAA:
				ip = rr.AAAA
			default:
				continue
			}

			if m.ipMatcher.Match(ip) {
				return true
			}
		}
	}

	return false
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	m, err := newResponseMatcher(tag, args)
	if err != nil {
		return nil, err
	}
	return handler.WrapMatcherPlugin(tag, PluginType, m), nil
}

func newResponseMatcher(tag string, args *Args) (m *responseMatcher, err error) {
	m = new(responseMatcher)
	m.tag = tag
	m.logger = mlog.NewPluginLogger(tag)
	m.args = args

	if len(args.Rcode) > 0 {
		m.rcodeMatcher = elem.NewIntMatcher(args.Rcode)
	}

	if len(args.CNAME) > 0 {
		m.cnameMatcher, err = domain.BatchLoadMixMatcherV2Matcher(args.CNAME)
		if err != nil {
			return nil, err
		}
	}

	if len(args.IP) > 0 {
		m.ipMatcher, err = netlist.BatchLoad(args.IP)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

func preset(tag string, args *Args) (m *responseMatcher) {
	m, err := newResponseMatcher(tag, args)
	if err != nil {
		panic(fmt.Sprintf("response_matcher: failed to init pre-set plugin %s: %s", tag, err))
	}
	return m
}
