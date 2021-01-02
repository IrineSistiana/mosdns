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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/domain"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/elem"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/netlist"
	"github.com/miekg/dns"
	"net"
)

type responseIPMatcher struct {
	ipMatcher netlist.Matcher
}

func newResponseIPMatcher(ipMatcher netlist.Matcher) *responseIPMatcher {
	return &responseIPMatcher{ipMatcher: ipMatcher}
}

func (m *responseIPMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	if qCtx.R == nil {
		return false, nil
	}

	for _, rr := range qCtx.R.Answer {
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
			return true, nil
		}
	}
	return false, nil
}

type cnameMatcher struct {
	domainMatcher domain.Matcher
}

func newCnameMatcher(domainMatcher domain.Matcher) *cnameMatcher {
	return &cnameMatcher{domainMatcher: domainMatcher}
}

func (m *cnameMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	if qCtx.R == nil {
		return false, nil
	}

	for _, rr := range qCtx.R.Answer {
		if cname, ok := rr.(*dns.CNAME); ok {
			if _, ok := m.domainMatcher.Match(cname.Target); ok {
				return true, nil
			}
		}
	}
	return false, nil
}

type rCodeMatcher struct {
	elemMatcher *elem.IntMatcher
}

func newRCodeMatcher(elemMatcher *elem.IntMatcher) *rCodeMatcher {
	return &rCodeMatcher{elemMatcher: elemMatcher}
}

func (m *rCodeMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	if qCtx.R == nil {
		return false, nil
	}
	if m.elemMatcher.Match(qCtx.R.Rcode) {
		return true, nil
	}
	return false, nil
}
