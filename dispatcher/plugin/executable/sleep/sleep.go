//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package sleep

import (
	"context"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/pool"
	"time"
)

const PluginType = "sleep"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*sleep)(nil)

type Args struct {
	Duration uint `yaml:"duration"` // (milliseconds) duration for sleep.
}

type sleep struct {
	*handler.BP

	d time.Duration
}

func (s *sleep) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	err := s.sleep(ctx)
	if err != nil {
		return err
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}

func (s *sleep) sleep(ctx context.Context) (err error) {
	if s.d <= 0 {
		return
	}

	timer := pool.GetTimer(s.d)
	defer pool.ReleaseTimer(timer)
	select {
	case <-timer.C:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	d := args.(*Args).Duration
	return &sleep{
		BP: bp,
		d:  time.Duration(d) * time.Millisecond,
	}, nil
}
