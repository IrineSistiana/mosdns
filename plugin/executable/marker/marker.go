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

package marker

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
)

const PluginType = "marker"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ coremain.ExecutablePlugin = (*markerPlugin)(nil)
var _ coremain.MatcherPlugin = (*markerPlugin)(nil)

type markerPlugin struct {
	*coremain.BP
	markId uint
}

func (s *markerPlugin) Match(_ context.Context, qCtx *query_context.Context) (matched bool, err error) {
	return qCtx.HasMark(s.markId), nil
}

type Args struct{}

// Exec implements handler.Executable.
func (s *markerPlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	qCtx.AddMark(s.markId)
	return executable_seq.ExecChainNode(ctx, qCtx, next)
}

func Init(bp *coremain.BP, _ interface{}) (p coremain.Plugin, err error) {
	markId, err := query_context.AllocateMark()
	if err != nil {
		return nil, err
	}
	return &markerPlugin{
		BP:     bp,
		markId: markId,
	}, nil
}
