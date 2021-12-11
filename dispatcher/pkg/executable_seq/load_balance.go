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
	loadBalance []handler.ExecutableChainNode

	logger *zap.Logger // not nil
	p      uint32
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

	pn := &LBNode{loadBalance: ps}

	if logger != nil {
		pn.logger = logger
	} else {
		pn.logger = zap.NewNop()
	}
	return pn, nil
}

func (n *LBNode) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	if len(n.loadBalance) > 0 {
		nextIdx := atomic.AddUint32(&n.p, 1) % uint32(len(n.loadBalance))
		if err := handler.ExecChainNode(ctx, qCtx, n.loadBalance[nextIdx]); err != nil {
			return fmt.Errorf("command sequence #%d: %w", nextIdx, err)
		}
	}
	return handler.ExecChainNode(ctx, qCtx, next)
}
