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
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
)

const PluginType = "sequence"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })

	MustRegQuickSetup("accept", setupAccept)
	MustRegQuickSetup("reject", setupReject)
	MustRegQuickSetup("return", setupReturn)
	MustRegQuickSetup("goto", setupGoto)
	MustRegQuickSetup("jump", setupJump)
}

type sequence struct {
	*coremain.BP

	chain            []*chainNode
	anonymousPlugins []any
}

func (s *sequence) Close() error {
	for _, plugin := range s.anonymousPlugins {
		closePlugin(plugin)
	}
	return nil
}

type Args = []RuleArgs

func Init(bp *coremain.BP, args interface{}) (coremain.Plugin, error) {
	return newSequencePlugin(bp, *args.(*Args))
}

func newSequencePlugin(bp *coremain.BP, ra []RuleArgs) (*sequence, error) {
	s := &sequence{
		BP: bp,
	}

	var rc []RuleConfig
	for _, ra := range ra {
		rc = append(rc, parseArgs(ra))
	}
	if err := s.buildChain(rc); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func (s *sequence) Exec(ctx context.Context, qCtx *query_context.Context) error {
	walker := newChainWalker(s.chain, nil)
	return walker.ExecNext(ctx, qCtx)
}
