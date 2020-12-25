//     Copyright (C) 2020, IrineSistiana
//
//     This file is part of mos-chinadns.
//
//     mos-chinadns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mos-chinadns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package cpool

import (
	"github.com/sirupsen/logrus"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func Test_Pool(t *testing.T) {
	conn, _ := net.Pipe()
	logger := logrus.NewEntry(logrus.StandardLogger())
	var cp *Pool
	cp = New(0, 0, time.Second, logger)
	if cp != nil {
		t.Fatal("cp should nil")
	}
	if cp.ConnRemain() != 0 {
		t.Fatal("nil cp should have 0 connection")
	}

	cp = New(8, time.Millisecond*500, time.Millisecond*250, logger)
	if c := cp.Get(); c != nil {
		t.Fatal("cp should be empty")
	}

	for i := 0; i < 8; i++ {
		cp.Put(conn)
	}
	if cp.pool.Len() != 8 {
		t.Fatal("cp should have 8 elems")
	}
	if atomic.LoadInt32(&cp.cleanerStatus) != cleanerOnline {
		t.Fatal("cp cleaner should be online")
	}
	cp.Put(conn) // if cp is full.
	if cp.pool.Len() != 8 {
		t.Fatalf("cp should have 8 elems, but got %d", cp.pool.Len())
	}
	if c := cp.Get(); c == nil {
		t.Fatal("cp should return a conn")
	}
	if cp.pool.Len() != 7 {
		t.Fatalf("cp should have 7 elems, but got %d", cp.pool.Len())
	}

	time.Sleep(time.Millisecond * 1000) // all elems are expired now.
	if cp.pool.Len() != 0 {             // all expired elems are removed
		t.Fatalf("cp should have 0 elems, but got %d", cp.pool.Len())
	}
	if atomic.LoadInt32(&cp.cleanerStatus) != cleanerOffline { // if no elem in pool, cleaner should exit.
		t.Fatal("cp cleaner should be offline")
	}
}
