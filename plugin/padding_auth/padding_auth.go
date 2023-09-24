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

package padding_auth

import (
	"bytes"
	"context"
	"crypto/md5"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const PluginType = "padding_auth"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, func(_ sequence.BQ, args string) (any, error) {
		return newPaddingAuth(args), nil
	})
	sequence.MustRegMatchQuickSetup(PluginType, func(_ sequence.BQ, args string) (sequence.Matcher, error) {
		return newPaddingAuth(args), nil
	})
}

var _ sequence.RecursiveExecutable = (*paddingAuth)(nil)
var _ sequence.Matcher = (*paddingAuth)(nil)

type paddingAuth struct {
	key []byte
}

func (m *paddingAuth) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	for _, optRR := range qCtx.QueryOpt {
		if padding, ok := optRR.(*dns.EDNS0_PADDING); ok {
			if bytes.Equal(padding.Padding, m.key) {
				return true, nil
			}
		}
	}

	return false, nil
}

func (m *paddingAuth) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	upgraded := m.addPadding(qCtx.Q())
	err := next.ExecNext(ctx, qCtx)
	if err != nil {
		return err
	}

	if r := qCtx.R(); r != nil {
		if upgraded {
			dnsutils.RemoveEDNS0(r)
		}
	}
	return nil
}

func (m *paddingAuth) addPadding(q *dns.Msg) (upgraded bool) {
	padding := &dns.EDNS0_PADDING{
		Padding: append([]byte(nil), m.key...),
	}

	opt := q.IsEdns0()
	if opt == nil {
		upgraded = true
		o := new(dns.OPT)
		o.SetUDPSize(dns.MinMsgSize)
		o.Hdr.Name = "."
		o.Hdr.Rrtype = dns.TypeOPT
		q.Extra = append(q.Extra, o)
		return
	}

	var overwritten bool
	for o := range opt.Option {
		if opt.Option[o].Option() == dns.EDNS0PADDING {
			opt.Option[o] = padding
			overwritten = true
			break
		}
	}
	if !overwritten {
		opt.Option = append(opt.Option, padding)
	}
	return
}

// newPaddingAuth format: str_key
func newPaddingAuth(s string) *paddingAuth {
	key := md5.Sum([]byte(s))
	return &paddingAuth{key: key[:]}
}
