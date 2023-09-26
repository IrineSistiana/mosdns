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

package rate_limiter

import (
	"context"
	"io"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/rate_limiter"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"golang.org/x/time/rate"
)

const PluginType = "rate_limiter"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

type Args struct {
	Qps   float64 `yaml:"qps"`
	Burst int     `yaml:"burst"`
	Mask4 int     `yaml:"mask4"`
	Mask6 int     `yaml:"mask6"`
}

func (args *Args) init() {
	utils.SetDefaultUnsignNum(&args.Qps, 20)
	utils.SetDefaultUnsignNum(&args.Burst, 40)
	utils.SetDefaultUnsignNum(&args.Mask4, 32)
	utils.SetDefaultUnsignNum(&args.Mask4, 48)
}

var _ sequence.Executable = (*RateLimiter)(nil)
var _ io.Closer = (*RateLimiter)(nil)

type RateLimiter struct {
	l rate_limiter.RateLimiter
}

func Init(_ *coremain.BP, args any) (any, error) {
	return New(*(args.(*Args))), nil
}

func New(args Args) *RateLimiter {
	args.init()
	l := rate_limiter.NewRateLimiter(rate.Limit(args.Qps), args.Burst, 0, args.Mask4, args.Mask6)
	return &RateLimiter{l: l}
}

func (s *RateLimiter) Exec(ctx context.Context, qCtx *query_context.Context) error {
	clientAddr := qCtx.ServerMeta.ClientAddr
	if clientAddr.IsValid() {
		if !s.l.Allow(clientAddr) {
			qCtx.SetResponse(refuse(qCtx.Q()))
		}
	}
	return nil
}

func (s *RateLimiter) Close() error {
	return s.l.Close()
}

func refuse(q *dns.Msg) *dns.Msg {
	r := new(dns.Msg)
	r.SetReply(q)
	r.Rcode = dns.RcodeRefused
	return r
}
