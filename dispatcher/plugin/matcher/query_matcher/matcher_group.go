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
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
)

type clientIPMatcher struct {
	ipMatcher netlist.Matcher
}

func newClientIPMatcher(ipMatcher netlist.Matcher) *clientIPMatcher {
	return &clientIPMatcher{ipMatcher: ipMatcher}
}

func (m *clientIPMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	if qCtx.From != nil {
		ip := utils.GetIPFromAddr(qCtx.From)
		if ip != nil {
			if m.ipMatcher.Match(ip) {
				return true, nil
			}
		} else {
			return false, fmt.Errorf("internal err: client addr [%s] is invalid", qCtx.From)
		}
	}
	return false, nil
}

type qDomainMatcher struct {
	domainMatcher domain.Matcher
}

func newQDomainMatcher(domainMatcher domain.Matcher) *qDomainMatcher {
	return &qDomainMatcher{domainMatcher: domainMatcher}
}

func (m *qDomainMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	for i := range qCtx.Q.Question {
		_, matched = m.domainMatcher.Match(qCtx.Q.Question[i].Name)
		if matched {
			return true, nil
		}
	}
	return false, nil
}

type qTypeMatcher struct {
	elemMatcher *elem.IntMatcher
}

func newQTypeMatcher(elemMatcher *elem.IntMatcher) *qTypeMatcher {
	return &qTypeMatcher{elemMatcher: elemMatcher}
}

func (m *qTypeMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	for i := range qCtx.Q.Question {
		if m.elemMatcher.Match(int(qCtx.Q.Question[i].Qtype)) {
			return true, nil
		}
	}
	return false, nil
}

type qClassMatcher struct {
	elemMatcher *elem.IntMatcher
}

func newQClassMatcher(elemMatcher *elem.IntMatcher) *qClassMatcher {
	return &qClassMatcher{elemMatcher: elemMatcher}
}

func (m *qClassMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	for i := range qCtx.Q.Question {
		if m.elemMatcher.Match(int(qCtx.Q.Question[i].Qclass)) {
			return true, nil
		}
	}
	return false, nil
}
