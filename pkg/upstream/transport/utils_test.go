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

package transport

import (
	"math/rand"
	"testing"
	"time"
)

func Test_sliceAdd(t *testing.T) {
	sLen := 1024
	var s []int
	var emptyS []int
	r := rand.New(rand.NewSource(time.Now().Unix()))
	fillS := func() {
		var ns []int
		for i := 0; i < sLen; i++ {
			sliceAdd(&ns, i)
		}
		s = ns
	}

	// test add
	fillS()
	for i := 0; i < sLen; i++ {
		if i != s[i] {
			t.Fatal("add error")
		}
	}

	// test del
	fillS()
	for i := 0; i < sLen; i++ {
		sliceDel(&s, 0)
	}
	if len(s) != 0 {
		t.Fatalf("del error")
	}

	// test random get
	fillS()
	for i := 0; i < sLen; i++ {
		n, v := sliceRandGet(s, r)
		if n != v {
			t.Fatal("random get failed")
		}
	}
	n, v := sliceRandGet(emptyS, r)
	if n != -1 || v != 0 {
		t.Fatal("random get on empty s failed")
	}

	// test random pop
	fillS()
	for i := 0; i < sLen; i++ {
		sliceRandPop(&s, r)
	}
	if len(s) != 0 {
		t.Fatalf("rand pop error")
	}
	n, ok := sliceRandPop(&emptyS, r)
	if n != 0 || ok {
		t.Fatal("random get on empty s failed")
	}

	// test pop latest
	fillS()
	for i := 0; i < sLen; i++ {
		n, ok := slicePopLatest(&s)
		if n != sLen-i-1 || !ok {
			t.Fatal("pop latest s failed")
		}
	}
	n, ok = slicePopLatest(&emptyS) // empty s
	if n != 0 || ok {
		t.Fatal("pop latest s failed")
	}
}
