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

package executable_seq

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"sync"
	"time"
)

func asyncWait(ctx context.Context, qCtx *query_context.Context, logger *zap.Logger, c chan *parallelECSResult, total int) error {
	for i := 0; i < total; i++ {
		select {
		case res := <-c:
			if res.err != nil {
				logger.Warn("sequence failed", qCtx.InfoField(), zap.Int("sequence", res.from), zap.Error(res.err))
				continue
			}

			if res.qCtx != nil && res.qCtx.R() != nil {
				logger.Debug("sequence returned a response", qCtx.InfoField(), zap.Int("sequence", res.from))
				*qCtx = *res.qCtx
				return nil
			}

			logger.Debug("sequence returned with an empty response", qCtx.InfoField(), zap.Int("sequence", res.from))
			continue

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// No response
	qCtx.SetResponse(nil, query_context.ContextStatusServerFailed)
	return errors.New("no response")
}

// LastNode returns the Latest node of chain of n.
func LastNode(n ExecutableChainNode) ExecutableChainNode {
	p := n
	for {
		if nn := p.Next(); nn == nil {
			return p
		} else {
			p = nn
		}
	}
}

func ExecChainNode(ctx context.Context, qCtx *query_context.Context, n ExecutableChainNode) error {
	if n == nil {
		return nil
	}

	// TODO: Error logging
	return n.Exec(ctx, qCtx, n.Next())
}

type DummyMatcher struct {
	Matched bool
	WantErr error
}

func (d *DummyMatcher) Match(_ context.Context, _ *query_context.Context) (matched bool, err error) {
	return d.Matched, d.WantErr
}

type DummyExecutable struct {
	sync.Mutex
	WantSkip  bool
	WantSleep time.Duration
	WantR     *dns.Msg
	WantErr   error
}

func (d *DummyExecutable) Exec(ctx context.Context, qCtx *query_context.Context, next ExecutableChainNode) error {
	if d.WantSleep != 0 {
		time.Sleep(d.WantSleep)
	}

	d.Lock()
	if d.WantSkip {
		d.Unlock()
		return nil
	}
	if err := d.WantErr; err != nil {
		d.Unlock()
		return err
	}
	if d.WantR != nil {
		qCtx.SetResponse(d.WantR, query_context.ContextStatusResponded)
	}
	d.Unlock()
	return ExecChainNode(ctx, qCtx, next)
}

func LogicalAndMatcherGroup(ctx context.Context, qCtx *query_context.Context, mg []Matcher) (matched bool, err error) {
	if len(mg) == 0 {
		return false, nil
	}
	for _, matcher := range mg {
		ok, err := matcher.Match(ctx, qCtx)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
