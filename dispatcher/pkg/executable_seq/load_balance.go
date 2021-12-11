//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) or later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package executable_seq

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"go.uber.org/zap"
	"sync/atomic"
)

type LBNode struct {
	prev, next handler.ExecutableChainNode
	branchNode []handler.ExecutableChainNode
	p          uint32
}

func (lbn *LBNode) Previous() handler.ExecutableChainNode {
	return lbn.prev
}

func (lbn *LBNode) Next() handler.ExecutableChainNode {
	return lbn.next
}

func (lbn *LBNode) LinkPrevious(n handler.ExecutableChainNode) {
	lbn.prev = n
	for _, branch := range lbn.branchNode {
		branch.LinkPrevious(n)
	}
}

func (lbn *LBNode) LinkNext(n handler.ExecutableChainNode) {
	lbn.next = n
	for _, branch := range lbn.branchNode {
		handler.LastNode(branch).LinkNext(n)
	}
}

type LBConfig struct {
	LoadBalance []interface{} `yaml:"load_balance"`
}

func ParseLBNode(c *LBConfig, logger *zap.Logger) (*LBNode, error) {
	ps := make([]handler.ExecutableChainNode, 0, len(c.LoadBalance))
	for i, subSequence := range c.LoadBalance {
		es, err := ParseExecutableNode(subSequence, logger)
		if err != nil {
			return nil, fmt.Errorf("invalid load balance command #%d: %w", i, err)
		}
		ps = append(ps, es)
	}

	return &LBNode{branchNode: ps}, nil
}

func (lbn *LBNode) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	if len(lbn.branchNode) == 0 {
		return handler.ExecChainNode(ctx, qCtx, next)
	}

	nextIdx := atomic.AddUint32(&lbn.p, 1) % uint32(len(lbn.branchNode))
	err := handler.ExecChainNode(ctx, qCtx, lbn.branchNode[nextIdx])
	if err != nil {
		return fmt.Errorf("command sequence #%d: %w", nextIdx, err)
	}
	return nil
}
