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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/elem"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/netlist"
	"github.com/miekg/dns"
	"net"
)

type ClientIPMatcher struct {
	ipMatcher netlist.Matcher
}

func NewClientIPMatcher(ipMatcher netlist.Matcher) *ClientIPMatcher {
	return &ClientIPMatcher{ipMatcher: ipMatcher}
}

func (m *ClientIPMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	var clientIP net.IP
	if meta := qCtx.ReqMeta(); meta != nil {
		clientIP = meta.ClientIP
	}
	if clientIP == nil {
		return false, nil
	}

	return m.ipMatcher.Match(clientIP), nil
}

type EdnsIPMatcher struct {
	ipMatcher netlist.Matcher
}

func NewEdnsIPMatcher(ipMatcher netlist.Matcher) *EdnsIPMatcher {
	return &EdnsIPMatcher{ipMatcher: ipMatcher}
}

func (m *EdnsIPMatcher) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	var ednsIP net.IP
	q := qCtx.Q()
	if ecs := dnsutils.GetMsgECS(q); ecs != nil {
		ednsIP = ecs.Address
		//dnsutils.RemoveMsgECS(qCtx.Q())
	}
	if ednsIP == nil {
		return false, nil
	}

	return m.ipMatcher.Match(ednsIP), nil
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
