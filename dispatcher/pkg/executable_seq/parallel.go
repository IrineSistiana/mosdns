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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"time"
)

type ParallelECS struct {
	s       []*ExecutableCmdSequence
	timeout time.Duration
}

type ParallelECSConfig struct {
	Parallel [][]interface{} `yaml:"parallel"`
	Timeout  uint            `yaml:"timeout"`
}

func ParseParallelECS(c *ParallelECSConfig) (*ParallelECS, error) {
	if len(c.Parallel) < 2 {
		return nil, fmt.Errorf("parallel needs at least 2 cmd sequences, but got %d", len(c.Parallel))
	}

	ps := make([]*ExecutableCmdSequence, 0, len(c.Parallel))
	for i, subSequence := range c.Parallel {
		es, err := ParseExecutableCmdSequence(subSequence)
		if err != nil {
			return nil, fmt.Errorf("invalid parallel sequence at index %d: %w", i, err)
		}
		ps = append(ps, es)
	}
	return &ParallelECS{s: ps, timeout: time.Duration(c.Timeout) * time.Second}, nil
}

type parallelECSResult struct {
	r      *dns.Msg
	status handler.ContextStatus
	err    error
	from   int
}

func (p *ParallelECS) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error) {
	return false, p.execCmd(ctx, qCtx, logger)
}

func (p *ParallelECS) execCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (err error) {

	var pCtx context.Context // only valid if p.timeout == 0
	var cancel func()
	if p.timeout == 0 {
		pCtx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	t := len(p.s)
	c := make(chan *parallelECSResult, len(p.s)) // use buf chan to avoid blocking.

	for i, sequence := range p.s {
		i := i
		sequence := sequence
		qCtxCopy := qCtx.Copy()

		go func() {
			var ecsCtx context.Context
			var ecsCancel func()
			if p.timeout == 0 {
				ecsCtx = pCtx
			} else {
				ecsCtx, ecsCancel = context.WithTimeout(context.Background(), p.timeout)
				defer ecsCancel()
			}

			err := ExecRoot(ecsCtx, qCtxCopy, logger, sequence)
			c <- &parallelECSResult{
				r:      qCtxCopy.R(),
				status: qCtxCopy.Status(),
				err:    err,
				from:   i,
			}
		}()
	}

	return asyncWait(ctx, qCtx, logger, c, t)
}
