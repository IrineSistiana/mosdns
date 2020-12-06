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

package simplematcher

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
)

const PluginType = "simple_matcher"

func init() {
	handler.MustRegPlugin(handler.WrapMatcherPlugin("_response_no_valid_ipv6", PluginType, &responseNoValidIPv6{}))
	handler.MustRegPlugin(handler.WrapMatcherPlugin("_response_err_rcode", PluginType, &responseErrRcode{}))
	handler.MustRegPlugin(handler.WrapMatcherPlugin("_query_unusual_types", PluginType, &queryUnusualTypes{}))
}

var _ handler.Matcher = (*responseNoValidIPv6)(nil)
var _ handler.Matcher = (*responseErrRcode)(nil)
var _ handler.Matcher = (*queryUnusualTypes)(nil)

type responseNoValidIPv6 struct{}

// Match returns true if qCtx.R question is AAAA type but has no valid AAAA answer.
// Match never returns an err.
func (m *responseNoValidIPv6) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	if qCtx == nil || qCtx.R == nil || len(qCtx.R.Question) != 1 {
		return false, nil
	}

	if qCtx.R.Question[0].Qtype == dns.TypeAAAA { // is AAAA query
		noIPv6 := true
		for i := range qCtx.R.Answer {
			if _, ok := qCtx.R.Answer[i].(*dns.AAAA); ok {
				noIPv6 = false
				break
			}
		}
		return noIPv6, nil
	}

	return false, nil
}

type responseErrRcode struct{}

// Match returns true if qCtx.R.Rcode != 0.
// Match never returns an err.
func (m *responseErrRcode) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	if qCtx == nil || qCtx.R == nil {
		return false, nil
	}

	return qCtx.R.Rcode != dns.RcodeSuccess, err
}

type queryUnusualTypes struct{}

// Match returns true if qCtx.Q is quite unusual, which means:
// 1. len(qCtx.Q.Question) != 1
// 2. qCtx.Q.Question[0].Qclass != dns.ClassINET
// 3. qCtx.Q.Question[0].Qtype is not dns.TypeA nor dns.TypeAAAA
// Match never returns an err.
func (m *queryUnusualTypes) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	if qCtx == nil || qCtx.Q == nil {
		return false, nil
	}

	return len(qCtx.Q.Question) != 1 || qCtx.Q.Question[0].Qclass != dns.ClassINET ||
		(qCtx.Q.Question[0].Qtype != dns.TypeA && qCtx.Q.Question[0].Qtype != dns.TypeAAAA), nil
}
