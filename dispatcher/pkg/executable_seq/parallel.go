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
	"time"
)

type ParallelNode struct {
	s       []handler.ExecutableChainNode
	timeout time.Duration

	logger *zap.Logger // not nil
}

const (
	defaultParallelTimeout = time.Second * 5
)

type ParallelConfig struct {
	Parallel []interface{} `yaml:"parallel"`
	Timeout  uint          `yaml:"timeout"`
}

func ParseParallelNode(c *ParallelConfig, logger *zap.Logger) (*ParallelNode, error) {
	ps := make([]handler.ExecutableChainNode, 0, len(c.Parallel))
	for i, subSequence := range c.Parallel {
		es, err := ParseExecutableNode(subSequence, logger)
		if err != nil {
			return nil, fmt.Errorf("invalid parallel command at index %d: %w", i, err)
		}
		ps = append(ps, es)
	}

	pn := &ParallelNode{s: ps, timeout: time.Duration(c.Timeout) * time.Second}

	if logger != nil {
		pn.logger = logger
	} else {
		pn.logger = zap.NewNop()
	}
	return pn, nil
}

type parallelECSResult struct {
	qCtx *handler.Context
	err  error
	from int
}

func (p *ParallelNode) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	if err := p.exec(ctx, qCtx); err != nil {
		return err
	}
	return handler.ExecChainNode(ctx, qCtx, next)
}

func (p *ParallelNode) exec(ctx context.Context, qCtx *handler.Context) error {
	if len(p.s) == 0 {
		return nil
	}

	t := len(p.s)
	c := make(chan *parallelECSResult, len(p.s)) // use buf chan to avoid blocking.

	for i, n := range p.s {
		i := i
		n := n
		qCtxCopy := qCtx.Copy()

		var pCtx context.Context
		var cancel func()
		if p.timeout > 0 {
			pCtx, cancel = context.WithTimeout(context.Background(), p.timeout)
		} else {
			if ddl, ok := ctx.Deadline(); ok {
				pCtx, cancel = context.WithDeadline(ctx, ddl)
			}
			pCtx, cancel = context.WithTimeout(ctx, defaultParallelTimeout)
		}

		go func() {
			defer cancel()

			err := handler.ExecChainNode(pCtx, qCtxCopy, n)
			c <- &parallelECSResult{
				qCtx: qCtxCopy,
				err:  err,
				from: i,
			}
		}()
	}

	return asyncWait(ctx, qCtx, p.logger, c, t)
}
