//     Copyright (C) 2020, IrineSistiana
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

type DummyMatcher struct {
	Matched bool
	WantErr error
}

func (d *DummyMatcher) Match(_ context.Context, _ *Context) (matched bool, err error) {
	return d.Matched, d.WantErr
}

type DummyExecutable struct {
	WantErr error
}

func (d *DummyExecutable) Exec(_ context.Context, _ *Context) (err error) {
	return d.WantErr
}

type DummyServerHandler struct {
	T       *testing.T
	EchoMsg *dns.Msg
	WantErr error
}

func (d *DummyServerHandler) ServeDNS(_ context.Context, qCtx *Context, w ResponseWriter) {
	r := d.EchoMsg.Copy()
	r.Id = qCtx.Q.Id
	_, err := w.Write(r)
	if err != nil {
		d.T.Errorf("DummyServerHandler: %v", err)
	}
}
