package concurrent_lru

import (
	"reflect"
	"strconv"
	"testing"
)

func TestConcurrentLRU(t *testing.T) {
	onEvict := func(key string, v interface{}) {}
	onGet := func(key string, v interface{}) interface{} {
		if v.(string) != key {
			t.Fatalf("kv pair mismatched: key: %s, v: %s", key, v)
		}
		return v
	}

	var lru *ConcurrentLRU
	reset := func() {
		lru = NewConcurrentLRU(4, 16, onEvict, onGet) // max size 64
	}

	add := func(keys ...string) {
		for _, key := range keys {
			lru.Add(key, key)
		}
	}

	mustGet := func(keys ...string) {
		for _, key := range keys {
			gotV, ok := lru.Get(key)
			if !ok || !reflect.DeepEqual(gotV, key) {
				t.Fatalf("want %v, got %v", key, gotV)
			}
		}
	}

	emptyGet := func(keys ...string) {
		for _, key := range keys {
			gotV, ok := lru.Get(key)
			if ok || gotV != nil {
				t.Fatalf("want empty, got %v", gotV)
			}
		}
	}

	checkLen := func(want int) {
		if want != lru.Len() {
			t.Fatalf("want %v, got %v", want, lru.Len())
		}
	}

	// test add
	reset()
	add("1", "1", "1", "1", "1", "2", "3")
	checkLen(3)
	mustGet("1", "2", "3")
	emptyGet("4", "5", "6")

	// test add overflow
	reset()
	for i := 0; i < 1024; i++ { // max size is 64
		add(strconv.Itoa(i))
	}
	if lru.Len() > 64 {
		t.Fatalf("lru overflowed: want len = %d, got = %d", 64, lru.Len())
	}

	// test del
	reset()
	add("1", "2", "3", "4")
	lru.Del("2")
	lru.Del("4")
	lru.Del("9999")
	mustGet("1", "3")
	emptyGet("2", "4")

	// test clean
	reset()
	add("1", "2", "3")
	cleanFunc := func(key string, v interface{}) (remove bool) {
		switch key {
		case "1", "3":
			return true
		}
		return false
	}
	if cleaned := lru.Clean(cleanFunc); cleaned != 2 {
		t.Fatalf("q.Clean want cleaned = 2, got %v", cleaned)
	}
	mustGet("2")
	emptyGet("1", "3")
}
