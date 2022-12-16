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

type actionAccept struct{}

func (a actionAccept) Exec(_ context.Context, _ *query_context.Context, _ ChainWalker) error {
	return nil
}

func setupAccept(_ BQ, _ string) (any, error) {
	return actionAccept{}, nil
}

type actionReject struct {
	rcode int
}

func (a actionReject) Exec(_ context.Context, qCtx *query_context.Context, _ ChainWalker) error {
	r := new(dns.Msg)
	r.SetReply(qCtx.Q())
	r.Rcode = a.rcode
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
	return actionReject{rcode: rcode}, nil
}

type actionReturn struct{}

func (a actionReturn) Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error {
	if next.jumpBack != nil {
		return next.jumpBack.ExecNext(ctx, qCtx)
	}
	return nil
}

func setupReturn(_ BQ, _ string) (any, error) {
	return actionReturn{}, nil
}

type actionJump struct {
	to *sequence
}

func (a *actionJump) Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error {
	w := newChainWalker(a.to.chain, &next)
	return w.ExecNext(ctx, qCtx)
}

func setupJump(bq BQ, s string) (any, error) {
	jumpTo, _ := bq.M().GetPlugin(s).(*sequence)
	if jumpTo == nil {
		return nil, fmt.Errorf("can not find jump target %s", s)
	}
	return &actionJump{to: jumpTo}, nil
}

type actionGoto struct {
	to *sequence
}

func (a actionGoto) Exec(ctx context.Context, qCtx *query_context.Context, _ ChainWalker) error {
	w := newChainWalker(a.to.chain, nil)
	return w.ExecNext(ctx, qCtx)
}

func setupGoto(bq BQ, s string) (any, error) {
	gt, _ := bq.M().GetPlugin(s).(*sequence)
	if gt == nil {
		return nil, fmt.Errorf("can not find goto target %s", s)
	}
	return &actionGoto{to: gt}, nil
}
