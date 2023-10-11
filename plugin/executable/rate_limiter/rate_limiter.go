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
	"fmt"
	"io"
	"net/netip"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/rate_limiter"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
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

func (args *Args) init() error {
	utils.SetDefaultUnsignNum(&args.Qps, 20)
	utils.SetDefaultUnsignNum(&args.Burst, 40)
	utils.SetDefaultUnsignNum(&args.Mask4, 32)
	utils.SetDefaultUnsignNum(&args.Mask6, 48)

	if !utils.CheckNumRange(args.Mask4, 0, 32) {
		return fmt.Errorf("invalid mask4")
	}
	if !utils.CheckNumRange(args.Mask6, 0, 128) {
		return fmt.Errorf("invalid mask6")
	}
	return nil
}

var _ sequence.Matcher = (*RateLimiter)(nil)
var _ io.Closer = (*RateLimiter)(nil)

type RateLimiter struct {
	args Args
	l    *rate_limiter.Limiter
}

func Init(_ *coremain.BP, args any) (any, error) {
	return New(*(args.(*Args)))
}

func New(args Args) (*RateLimiter, error) {
	err := args.init()
	if err != nil {
		return nil, fmt.Errorf("invalid args, %w", err)
	}
	l := rate_limiter.NewRateLimiter(rate.Limit(args.Qps), args.Burst)
	return &RateLimiter{l: l, args: args}, nil
}

func (s *RateLimiter) Match(ctx context.Context, qCtx *query_context.Context) (bool, error) {
	addr := s.getMaskedClientAddr(qCtx)
	if addr.IsValid() {
		return s.l.Allow(addr), nil
	}
	return true, nil
}

func (s *RateLimiter) getMaskedClientAddr(qCtx *query_context.Context) netip.Addr {
	a := qCtx.ServerMeta.ClientAddr
	if !a.IsValid() {
		return netip.Addr{}
	}
	a = a.Unmap()
	var p netip.Prefix
	if a.Is4() {
		p, _ = a.Prefix(s.args.Mask4)
	} else {
		p, _ = a.Prefix(s.args.Mask6)
	}
	return p.Addr()
}

func (s *RateLimiter) Close() error {
	return s.l.Close()
}
