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

package concurrent_lru

import (
	"reflect"
	"testing"
)

type testKey int

func (k testKey) Sum() uint64 {
	return uint64(k)
}

func TestConcurrentLRU(t *testing.T) {
	onEvict := func(k testKey, v int) {}

	var cache *ShardedLRU[testKey, int]
	reset := func(shardNum, maxShardSize int) {
		cache = NewShardedLRU[testKey, int](shardNum, maxShardSize, onEvict)
	}

	add := func(keys ...int) {
		for _, k := range keys {
			cache.Add(testKey(k), k)
		}
	}

	mustGet := func(keys ...int) {
		for _, k := range keys {
			gotV, ok := cache.Get(testKey(k))
			if !ok || !reflect.DeepEqual(gotV, k) {
				t.Fatalf("want %v, got %v", k, gotV)
			}
		}
	}

	emptyGet := func(keys ...int) {
		for _, k := range keys {
			gotV, ok := cache.Get(testKey(k))
			if ok {
				t.Fatalf("want empty, got %v", gotV)
			}
		}
	}

	checkLen := func(want int) {
		if want != cache.Len() {
			t.Fatalf("want %v, got %v", want, cache.Len())
		}
	}

	// test add
	reset(4, 16)
	add(1, 1, 1, 1, 2, 2, 3, 3, 4)
	checkLen(4)
	mustGet(1, 2, 3, 4)
	emptyGet(5, 6, 7, 9999)

	// test add overflow
	reset(4, 16) // max size is 64
	for i := 0; i < 1024; i++ {
		add(i)
	}
	if cache.Len() > 64 {
		t.Fatalf("lru overflowed: want len = %d, got = %d", 64, cache.Len())
	}

	// test del
	reset(4, 16)
	add(1, 2, 3, 4)
	cache.Del(2)
	cache.Del(4)
	cache.Del(9999)
	mustGet(1, 3)
	emptyGet(2, 4)

	// test clean
	reset(4, 16)
	add(1, 2, 3, 4)
	cleanFunc := func(k testKey, v int) (remove bool) {
		switch k {
		case 1, 3:
			return true
		}
		return false
	}
	if cleaned := cache.Clean(cleanFunc); cleaned != 2 {
		t.Fatalf("q.Clean want cleaned = 2, got %v", cleaned)
	}
	mustGet(2, 4)
	emptyGet(1, 3)
}
