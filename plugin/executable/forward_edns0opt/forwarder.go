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

package forwardedns0opt

import (
	"context"
	"strconv"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
)

const PluginType = "forward_edns0opt"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.RecursiveExecutable = (*forwarder)(nil)

type forwarder struct {
	forwardTypCodes map[uint32]struct{}
}

func (f *forwarder) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	qOpt := qCtx.QOpt()
	clientOpt := qCtx.ClientOpt()
	if clientOpt != nil {
		for _, o := range clientOpt.Option {
			if _, ok := f.forwardTypCodes[uint32(o.Option())]; ok {
				qOpt.Option = append(qOpt.Option, o)
			}
		}
	}

	err := next.ExecNext(ctx, qCtx)
	if err != nil {
		return err
	}

	upstreamOpt := qCtx.UpstreamOpt()
	respOpt := qCtx.RespOpt()
	if upstreamOpt != nil && respOpt != nil {
		for _, o := range upstreamOpt.Option {
			if _, ok := f.forwardTypCodes[uint32(o.Option())]; ok {
				respOpt.Option = append(respOpt.Option, o)
			}
		}
	}
	return nil
}

// Format: [DNS EDNS0 Option Code] ...
func QuickSetup(_ sequence.BQ, numbers string) (any, error) {
	m := make(map[uint32]struct{})
	for _, s := range strings.Fields(numbers) {
		n, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return nil, err
		}
		m[uint32(n)] = struct{}{}
	}
	return &forwarder{forwardTypCodes: m}, nil
}
