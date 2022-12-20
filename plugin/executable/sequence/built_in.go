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

package sequence

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
	"strconv"
)

var _ RecursiveExecutable = (*ActionAccept)(nil)

type ActionAccept struct{}

func (a ActionAccept) Exec(_ context.Context, _ *query_context.Context, _ ChainWalker) error {
	return nil
}

func setupAccept(_ BQ, _ string) (any, error) {
	return ActionAccept{}, nil
}

var _ RecursiveExecutable = (*ActionReject)(nil)

type ActionReject struct {
	Rcode int
}

func (a ActionReject) Exec(_ context.Context, qCtx *query_context.Context, _ ChainWalker) error {
	r := new(dns.Msg)
	r.SetReply(qCtx.Q())
	r.Rcode = a.Rcode
	qCtx.SetResponse(r)
	return nil
}

func setupReject(_ BQ, s string) (any, error) {
	rcode := dns.RcodeRefused
	if len(s) > 0 {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 0xFFF {
			return nil, fmt.Errorf("invalid rcode [%s]", s)
		}
		rcode = n
	}
	return ActionReject{Rcode: rcode}, nil
}

var _ RecursiveExecutable = (*ActionReturn)(nil)

type ActionReturn struct{}

func (a ActionReturn) Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error {
	if next.jumpBack != nil {
		return next.jumpBack.ExecNext(ctx, qCtx)
	}
	return nil
}

func setupReturn(_ BQ, _ string) (any, error) {
	return ActionReturn{}, nil
}

var _ RecursiveExecutable = (*ActionJump)(nil)

type ActionJump struct {
	To []*ChainNode
}

func (a *ActionJump) Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error {
	w := NewChainWalker(a.To, &next)
	return w.ExecNext(ctx, qCtx)
}

func setupJump(bq BQ, s string) (any, error) {
	target, _ := bq.M().GetPlugin(s).(*Sequence)
	if target == nil {
		return nil, fmt.Errorf("can not find jump target %s", s)
	}
	return &ActionJump{To: target.chain}, nil
}

var _ RecursiveExecutable = (*ActionGoto)(nil)

type ActionGoto struct {
	To []*ChainNode
}

func (a ActionGoto) Exec(ctx context.Context, qCtx *query_context.Context, _ ChainWalker) error {
	w := NewChainWalker(a.To, nil)
	return w.ExecNext(ctx, qCtx)
}

func setupGoto(bq BQ, s string) (any, error) {
	gt, _ := bq.M().GetPlugin(s).(*Sequence)
	if gt == nil {
		return nil, fmt.Errorf("can not find goto target %s", s)
	}
	return &ActionGoto{To: gt.chain}, nil
}

var _ Matcher = (*MatchAlwaysTrue)(nil)

type MatchAlwaysTrue struct{}

func (m MatchAlwaysTrue) Match(_ context.Context, _ *query_context.Context) (bool, error) {
	return true, nil
}

func setupTrue(_ BQ, _ string) (Matcher, error) {
	return MatchAlwaysTrue{}, nil
}

var _ Matcher = (*MatchAlwaysFalse)(nil)

type MatchAlwaysFalse struct{}

func (m MatchAlwaysFalse) Match(_ context.Context, _ *query_context.Context) (bool, error) {
	return false, nil
}

func setupFalse(_ BQ, _ string) (Matcher, error) {
	return MatchAlwaysFalse{}, nil
}
