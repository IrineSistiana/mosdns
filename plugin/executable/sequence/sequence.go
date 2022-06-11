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
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
)

const PluginType = "sequence"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
	coremain.RegNewPersetPluginFunc("_return", func(bp *coremain.BP) (coremain.Plugin, error) {
		return &_return{BP: bp}, nil
	})
}

type sequence struct {
	*coremain.BP

	ecs executable_seq.ExecutableChainNode
}

type Args struct {
	Exec interface{} `yaml:"exec"`
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newSequencePlugin(bp, args.(*Args))
}

func newSequencePlugin(bp *coremain.BP, args *Args) (*sequence, error) {
	ecs, err := executable_seq.BuildExecutableLogicTree(args.Exec, bp.L(), bp.M().GetExecutables(), bp.M().GetMatchers())
	if err != nil {
		return nil, fmt.Errorf("cannot build sequence: %w", err)
	}

	return &sequence{
		BP:  bp,
		ecs: ecs,
	}, nil
}

func (s *sequence) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	if err := executable_seq.ExecChainNode(ctx, qCtx, s.ecs); err != nil {
		return err
	}

	return executable_seq.ExecChainNode(ctx, qCtx, next)
}

var _ coremain.ExecutablePlugin = (*_return)(nil)

type _return struct {
	*coremain.BP
}

func (n *_return) Exec(_ context.Context, _ *query_context.Context, _ executable_seq.ExecutableChainNode) error {
	return nil
}
