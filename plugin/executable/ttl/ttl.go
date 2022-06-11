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

package ttl

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
)

const (
	PluginType = "ttl"
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ coremain.ExecutablePlugin = (*ttl)(nil)

type Args struct {
	MaximumTTL uint32 `yaml:"maximum_ttl"`
	MinimalTTL uint32 `yaml:"minimal_ttl"`
}

type ttl struct {
	*coremain.BP
	args *Args
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newTTL(bp, args.(*Args)), nil
}

func newTTL(bp *coremain.BP, args *Args) coremain.Plugin {
	return &ttl{
		BP:   bp,
		args: args,
	}
}

func (t *ttl) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	if r := qCtx.R(); r != nil {
		if t.args.MaximumTTL > 0 {
			dnsutils.ApplyMaximumTTL(r, t.args.MaximumTTL)
		}
		if t.args.MinimalTTL > 0 {
			dnsutils.ApplyMinimalTTL(r, t.args.MinimalTTL)
		}
	}
	return executable_seq.ExecChainNode(ctx, qCtx, next)
}
