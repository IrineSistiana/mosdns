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

package msg_matcher

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/elem"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"net"
)

type ClientIPMatcher struct {
	ipMatcher netlist.Matcher
}

func NewClientIPMatcher(ipMatcher netlist.Matcher) *ClientIPMatcher {
	return &ClientIPMatcher{ipMatcher: ipMatcher}
}

var errNoClientAddr = errors.New("failed to get client address")

func (m *ClientIPMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	var addr net.Addr
	if meta := qCtx.ReqMeta(); meta != nil {
		addr = meta.From
	}
	if addr == nil {
		return false, errNoClientAddr
	}

	ip := utils.GetIPFromAddr(addr)
	if ip == nil {
		return false, fmt.Errorf("client addr [%s] is invalid", addr)
	}
	return m.ipMatcher.Match(ip), nil
}

type QNameMatcher struct {
	domainMatcher domain.Matcher
}

func NewQNameMatcher(domainMatcher domain.Matcher) *QNameMatcher {
	return &QNameMatcher{domainMatcher: domainMatcher}
}

func (m *QNameMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	return m.MatchMsg(qCtx.Q()), nil
}

func (m *QNameMatcher) MatchMsg(msg *dns.Msg) bool {
	for i := range msg.Question {
		_, ok := m.domainMatcher.Match(msg.Question[i].Name)
		if ok {
			return true
		}
	}
	return false
}

type QTypeMatcher struct {
	elemMatcher *elem.IntMatcher
}

func NewQTypeMatcher(elemMatcher *elem.IntMatcher) *QTypeMatcher {
	return &QTypeMatcher{elemMatcher: elemMatcher}
}

func (m *QTypeMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	return m.MatchMsg(qCtx.Q()), nil
}

func (m *QTypeMatcher) MatchMsg(msg *dns.Msg) bool {
	for i := range msg.Question {
		if m.elemMatcher.Match(int(msg.Question[i].Qtype)) {
			return true
		}
	}
	return false
}

type QClassMatcher struct {
	elemMatcher *elem.IntMatcher
}

func NewQClassMatcher(elemMatcher *elem.IntMatcher) *QClassMatcher {
	return &QClassMatcher{elemMatcher: elemMatcher}
}

func (m *QClassMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, _ error) {
	return m.MatchMsg(qCtx.Q()), nil
}

func (m *QClassMatcher) MatchMsg(msg *dns.Msg) bool {
	for i := range msg.Question {
		if m.elemMatcher.Match(int(msg.Question[i].Qclass)) {
			return true
		}
	}
	return false
}
