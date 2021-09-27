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

package handler

import (
	"context"
	"github.com/miekg/dns"
	"time"
)

// Types and funcs in this file are for testing only

type DummyMatcherPlugin struct {
	*BP
	Matched bool
	WantErr error
}

func (d *DummyMatcherPlugin) Match(_ context.Context, _ *Context) (matched bool, err error) {
	return d.Matched, d.WantErr
}

type DummyExecutablePlugin struct {
	*BP
	WantSkip  bool
	WantSleep time.Duration
	WantR     *dns.Msg
	WantErr   error
}

func (d *DummyExecutablePlugin) Exec(ctx context.Context, qCtx *Context, next ExecutableChainNode) error {
	if d.WantSkip {
		return nil
	}

	if d.WantSleep != 0 {
		time.Sleep(d.WantSleep)
	}

	if d.WantErr != nil {
		return d.WantErr
	}
	if d.WantR != nil {
		qCtx.SetResponse(d.WantR, ContextStatusResponded)
	}

	return ExecChainNode(ctx, qCtx, next)
}

type DummyServicePlugin struct {
	*BP
	WantShutdownErr error
}

func (d *DummyServicePlugin) Shutdown() error {
	return d.WantShutdownErr
}
