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

package msg_matcher

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/elem"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"net"
)

type AAAAAIPMatcher struct {
	ipMatcher netlist.Matcher
}

func NewAAAAAIPMatcher(ipMatcher netlist.Matcher) *AAAAAIPMatcher {
	return &AAAAAIPMatcher{ipMatcher: ipMatcher}
}

func (m *AAAAAIPMatcher) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	r := qCtx.R()
	if r == nil {
		return false, nil
	}

	return m.MatchMsg(r)
}

func (m *AAAAAIPMatcher) MatchMsg(msg *dns.Msg) (bool, error) {
	for _, rr := range msg.Answer {
		var ip net.IP
		switch rr := rr.(type) {
		case *dns.A:
			ip = rr.A
		case *dns.AAAA:
			ip = rr.AAAA
		default:
			continue
		}
		matched, err := m.ipMatcher.Match(ip)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

type CNameMatcher struct {
	domainMatcher domain.Matcher[struct{}]
}

func NewCNameMatcher(domainMatcher domain.Matcher[struct{}]) *CNameMatcher {
	return &CNameMatcher{domainMatcher: domainMatcher}
}

func (m *CNameMatcher) Match(_ context.Context, qCtx *query_context.Context) (matched bool, _ error) {
	r := qCtx.R()
	if r == nil {
		return false, nil
	}

	return m.MatchMsg(r), nil
}

func (m *CNameMatcher) MatchMsg(msg *dns.Msg) bool {
	for _, rr := range msg.Answer {
		if cname, ok := rr.(*dns.CNAME); ok {
			if _, ok := m.domainMatcher.Match(cname.Target); ok {
				return true
			}
		}
	}
	return false
}

type RCodeMatcher struct {
	elemMatcher *elem.IntMatcher
}

func NewRCodeMatcher(elemMatcher *elem.IntMatcher) *RCodeMatcher {
	return &RCodeMatcher{elemMatcher: elemMatcher}
}

func (m *RCodeMatcher) Match(_ context.Context, qCtx *query_context.Context) (matched bool, _ error) {
	r := qCtx.R()
	if r == nil {
		return false, nil
	}
	return m.MatchMsg(r), nil
}

func (m *RCodeMatcher) MatchMsg(msg *dns.Msg) bool {
	return m.elemMatcher.Match(msg.Rcode)
}
