//     Copyright (C) 2020-2021, IrineSistiana
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
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"net"
	"testing"
	"time"
)

func Test_Pool(t *testing.T) {
	conn, _ := net.Pipe()

	cp := New(8, time.Millisecond*200, time.Millisecond*100, mlog.L())
	if c := cp.Get(); c != nil {
		t.Fatal("cp should be empty")
	}

	for i := 0; i < 8; i++ {
		cp.Put(conn)
	}
	if cp.pool.Len() != 8 {
		t.Fatal("cp should have 8 elems")
	}
	if cp.cleanerStatus != cleanerOnline {
		t.Fatal("cp cleaner should be online")
	}
	cp.Put(conn) // if cp is full.
	if cp.ConnRemain() != 8 {
		t.Fatalf("cp should have 8 elems, but got %d", cp.pool.Len())
	}
	if c := cp.Get(); c == nil {
		t.Fatal("cp should return a conn")
	}
	if cp.ConnRemain() != 7 {
		t.Fatalf("cp should have 7 elems, but got %d", cp.pool.Len())
	}

	time.Sleep(time.Millisecond * 300) // all elems are expired now.
	if cp.ConnRemain() != 0 {          // all expired elems are removed
		t.Fatalf("cp should have 0 elems, but got %d", cp.pool.Len())
	}
	if cp.cleanerStatus != cleanerOffline { // if no elem in pool, cleaner should exit.
		t.Fatal("cp cleaner should be offline")
	}

	for i := 0; i < 8; i++ {
		cp.Put(conn)
	}
	time.Sleep(time.Millisecond * 300) // all elems are expired now.
	if c := cp.Get(); c != nil {       // Get() will should remove all connections.
		t.Fatal("cp should not return a conn")
	}
	if cp.ConnRemain() != 0 {
		t.Fatalf("cp should have 0 elems, but got %d", cp.pool.Len())
	}
}
