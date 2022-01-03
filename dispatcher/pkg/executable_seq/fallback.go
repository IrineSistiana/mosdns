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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/pool"
	"go.uber.org/zap"
	"sync"
	"time"
)

type FallbackConfig struct {
	// Primary exec sequence.
	Primary interface{} `yaml:"primary"`
	// Secondary exec sequence.
	Secondary interface{} `yaml:"secondary"`

	StatLength int `yaml:"stat_length"` // An Zero value disables the normal fallback.
	Threshold  int `yaml:"threshold"`

	// FastFallback threshold in milliseconds. Zero means fast fallback is disabled.
	FastFallback int `yaml:"fast_fallback"`

	// AlwaysStandby: secondary should always standby in fast fallback.
	AlwaysStandby bool `yaml:"always_standby"`
}

type FallbackNode struct {
	primary              handler.ExecutableChainNode
	secondary            handler.ExecutableChainNode
	fastFallbackDuration time.Duration
	alwaysStandby        bool

	primaryST *statusTracker // nil if normal fallback is disabled
	logger    *zap.Logger    // not nil
}

type statusTracker struct {
	sync.Mutex
	threshold int
	status    []uint8 // 0 means success, !0 means failed
	p         int
	s         int
	f         int
}

func newStatusTracker(threshold, statLength int) *statusTracker {
	return &statusTracker{
		threshold: threshold,
		status:    make([]uint8, statLength),
		s:         statLength,
	}
}

func (t *statusTracker) good() bool {
	t.Lock()
	defer t.Unlock()
	return t.f < t.threshold
}

func (t *statusTracker) update(s uint8) {
	t.Lock()
	defer t.Unlock()

	if s > 0 {
		t.f++
	} else {
		t.s++
	}

	if t.p >= len(t.status) {
		t.p = 0
	}
	oldS := t.status[t.p]
	if oldS > 0 {
		t.f--
	} else {
		t.s--
	}

	t.status[t.p] = s
	t.p++
}

func ParseFallbackNode(c *FallbackConfig, logger *zap.Logger) (*FallbackNode, error) {
	if c.Primary == nil {
		return nil, errors.New("primary is empty")
	}
	if c.Secondary == nil {
		return nil, errors.New("secondary is empty")
	}

	primaryECS, err := ParseExecutableNode(c.Primary, logger)
	if err != nil {
		return nil, fmt.Errorf("invalid primary sequence: %w", err)
	}

	secondaryECS, err := ParseExecutableNode(c.Secondary, logger)
	if err != nil {
		return nil, fmt.Errorf("invalid secondary sequence: %w", err)
	}

	fallbackECS := &FallbackNode{
		primary:              primaryECS,
		secondary:            secondaryECS,
		fastFallbackDuration: time.Duration(c.FastFallback) * time.Millisecond,
		alwaysStandby:        c.AlwaysStandby,
	}

	if c.StatLength > 0 {
		if c.Threshold > c.StatLength {
			c.Threshold = c.StatLength
		}
		fallbackECS.primaryST = newStatusTracker(c.Threshold, c.StatLength)
	}

	if logger != nil {
		fallbackECS.logger = logger
	} else {
		fallbackECS.logger = zap.NewNop()
	}

	return fallbackECS, nil
}

func (f *FallbackNode) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	if err := f.exec(ctx, qCtx); err != nil {
		return err
	}
	return handler.ExecChainNode(ctx, qCtx, next)
}

func (f *FallbackNode) exec(ctx context.Context, qCtx *handler.Context) error {
	if f.primaryST == nil || f.primaryST.good() {
		if f.fastFallbackDuration > 0 {
			return f.doFastFallback(ctx, qCtx)
		} else {
			return f.doPrimary(ctx, qCtx)
		}
	}
	f.logger.Debug("primary is not good", qCtx.InfoField())
	return f.doFallback(ctx, qCtx)
}

func (f *FallbackNode) isolateDoPrimary(ctx context.Context, qCtx *handler.Context) (err error) {
	qCtxCopy := qCtx.Copy()
	err = f.doPrimary(ctx, qCtxCopy)
	qCtx.SetResponse(qCtxCopy.R(), qCtxCopy.Status())
	return err
}

func (f *FallbackNode) doPrimary(ctx context.Context, qCtx *handler.Context) (err error) {
	err = handler.ExecChainNode(ctx, qCtx, f.primary)
	if f.primaryST != nil {
		if err != nil || qCtx.R() == nil {
			f.primaryST.update(1)
		} else {
			f.primaryST.update(0)
		}
	}

	return err
}

func (f *FallbackNode) doFastFallback(ctx context.Context, qCtx *handler.Context) (err error) {
	fCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	timer := pool.GetTimer(f.fastFallbackDuration)
	defer pool.ReleaseTimer(timer)

	c := make(chan *parallelECSResult, 2)
	primFailed := make(chan struct{}) // will be closed if primary returns an err.

	qCtxP := qCtx.Copy()
	go func() {
		err := f.doPrimary(fCtx, qCtxP)
		if err != nil || qCtxP.R() == nil {
			close(primFailed)
		}
		c <- &parallelECSResult{
			qCtx: qCtxP,
			err:  err,
			from: 1,
		}
	}()

	qCtxS := qCtx.Copy()
	go func() {
		if !f.alwaysStandby { // not always standby, wait here.
			select {
			case <-fCtx.Done(): // primary is done, no need to exec this.
				return
			case <-primFailed: // primary failed or timeout, exec now.
			case <-timer.C:
			}
		}

		err := f.doSecondary(fCtx, qCtxS)
		res := &parallelECSResult{
			qCtx: qCtxS,
			err:  err,
			from: 2,
		}

		if f.alwaysStandby { // always standby
			select {
			case <-fCtx.Done():
				return
			case <-primFailed: // only send secondary result when primary is failed.
				c <- res
			case <-timer.C: // or timeout.
				c <- res
			}
		} else {
			c <- res // not always standby, send the result asap.
		}
	}()

	return asyncWait(ctx, qCtx, f.logger, c, 2)
}

func (f *FallbackNode) doSecondary(ctx context.Context, qCtx *handler.Context) (err error) {
	return handler.ExecChainNode(ctx, qCtx, f.secondary)
}

func (f *FallbackNode) doFallback(ctx context.Context, qCtx *handler.Context) error {
	fCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	c := make(chan *parallelECSResult, 2) // buf size is 2, avoid blocking.

	qCtxP := qCtx.Copy()
	go func() {
		err := f.doPrimary(fCtx, qCtxP)
		c <- &parallelECSResult{
			qCtx: qCtxP,
			err:  err,
			from: 1,
		}
	}()

	qCtxS := qCtx.Copy()
	go func() {
		err := f.doSecondary(fCtx, qCtxS)
		c <- &parallelECSResult{
			qCtx: qCtxS,
			err:  err,
			from: 2,
		}
	}()

	return asyncWait(ctx, qCtx, f.logger, c, 2)
}
