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

package plainserver

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"testing"
	"time"
)

type testEchoHandler struct{}

func (t *testEchoHandler) ServeDNS(_ context.Context, qCtx *handler.Context, w handler.ResponseWriter) {
	_, err := w.Write(qCtx.Q)
	if err != nil {
		panic(err.Error())
	}
}

func newDummyMsg() *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	return m
}

func checkDummyMsg(t *testing.T, m *dns.Msg) {
	if len(m.Question) != 1 || m.Question[0].Name != "example.com." {
		t.Fatal("dummy msg assertion failed")
	}
}

func testServer(t *testing.T, c *dns.Conn) {
	for i := 0; i < 50; i++ {
		q := newDummyMsg()
		err := c.WriteMsg(q)
		if err != nil {
			t.Fatal(err)
		}

		c.SetReadDeadline(time.Now().Add(time.Second))
		r, err := c.ReadMsg()
		if err != nil {
			t.Fatal(err)
		}

		checkDummyMsg(t, r)
	}
}
