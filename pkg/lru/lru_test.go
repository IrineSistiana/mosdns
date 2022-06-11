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

package lru

import (
	"reflect"
	"testing"
)

func Test_lru(t *testing.T) {
	onEvict := func(key string, v interface{}) {}
	var q *LRU
	reset := func(maxSize int) {
		q = NewLRU(maxSize, onEvict)
	}

	add := func(keys ...string) {
		for _, key := range keys {
			q.Add(key, key)
		}
	}

	mustGet := func(keys ...string) {
		for _, key := range keys {
			gotV, ok := q.Get(key)
			if !ok || !reflect.DeepEqual(gotV, key) {
				t.Fatalf("want %v, got %v", key, gotV)
			}
		}
	}

	emptyGet := func(keys ...string) {
		for _, key := range keys {
			gotV, ok := q.Get(key)
			if ok || gotV != nil {
				t.Fatalf("want empty, got %v", gotV)
			}
		}
	}

	mustPop := func(keys ...string) {
		for _, key := range keys {
			gotKey, gotV, ok := q.PopOldest()
			if !ok {
				t.Fatal()
			}
			if gotKey != key || !reflect.DeepEqual(gotV, key) {
				t.Fatalf("want key: %v, v: %v, got key: %v, v:%v", key, key, gotKey, gotV)
			}
		}
	}

	emptyPop := func() {
		gotKey, gotV, ok := q.PopOldest()
		if ok {
			t.Fatal()
		}
		if gotKey != "" || gotV != nil {
			t.Fatalf("want empty result, got key: %v, v:%v", gotKey, gotV)
		}
	}

	checkLen := func(want int) {
		if q.l.Len() != len(q.m) {
			t.Fatalf("possible mem leak: q.l.Len() %v != len(q.m){ %v", q.l.Len(), len(q.m))
		}
		if want != q.Len() {
			t.Fatalf("want %v, got %v", want, q.Len())
		}
	}

	// test add
	reset(4)
	add("1", "1", "1", "1", "1", "2", "3")
	checkLen(3)
	mustGet("1", "2", "3")

	// test add overflow
	reset(2)
	add("1", "2", "3", "4")
	checkLen(2)
	mustGet("3", "4")
	emptyGet("1", "2")

	// test pop
	reset(3)
	add("1", "2", "3")
	mustPop("1", "2", "3")
	checkLen(0)
	emptyPop()

	// test del
	reset(3)
	add("1", "2", "3")
	q.Del("2")
	q.Del("9999")
	mustPop("1", "3")

	// test clean
	reset(3)
	add("1", "2", "3")
	cleanFunc := func(key string, v interface{}) (remove bool) {
		switch key {
		case "1", "3":
			return true
		}
		return false
	}
	if cleaned := q.Clean(cleanFunc); cleaned != 2 {
		t.Fatalf("q.Clean want cleaned = 2, got %v", cleaned)
	}
	mustPop("2")

	// test lru
	reset(4)
	add("1", "2", "3", "4") // 1 2 3 4
	mustGet("2", "3")       // 1 4 2 3
	mustPop("1", "4", "2", "3")
}
