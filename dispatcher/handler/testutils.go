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
	"testing"
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
	WantR   *dns.Msg
	WantErr error
}

func (d *DummyExecutablePlugin) Exec(_ context.Context, qCtx *Context) (err error) {
	if d.WantErr != nil {
		return d.WantErr
	}
	if d.WantR != nil {
		qCtx.SetResponse(d.WantR, ContextStatusResponded)
	}
	return nil
}

type DummyServicePlugin struct {
	*BP
	WantShutdownErr error
}

func (d *DummyServicePlugin) Shutdown() error {
	return d.WantShutdownErr
}

type DummyServerHandler struct {
	T       *testing.T
	WantMsg *dns.Msg
	WantErr error
}

func (d *DummyServerHandler) ServeDNS(_ context.Context, qCtx *Context, w ResponseWriter) {
	var r *dns.Msg
	if d.WantMsg != nil {
		r = d.WantMsg.Copy()
		r.Id = qCtx.Q().Id
	} else {
		r = new(dns.Msg)
		r.SetReply(qCtx.Q())
	}

	_, err := w.Write(r)
	if err != nil {
		d.T.Errorf("DummyServerHandler: %v", err)
	}
}
