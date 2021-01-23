package cache

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"
)

func Test_memCache(t *testing.T) {
	ctx := context.Background()

	c := newMemCache(8, time.Millisecond*50)
	for i := 0; i < 8; i++ {
		if err := c.store(ctx, strconv.Itoa(i), []byte{byte(i)}, time.Millisecond*200); err != nil {
			t.Fatal(err)
		}
	}

	if c.cleanerIsRunning != true {
		t.Fatal("cleaner goroutine should be online")
	}

	for i := 0; i < 8; i++ {
		v, _, ok, err := c.get(ctx, strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal()
		}
		if !bytes.Equal([]byte{byte(i)}, v) {
			t.Fatal("cache kv mismatched")
		}
	}
	if c.len() != 8 {
		t.Fatal()
	}

	for i := 8; i < 16; i++ {
		if err := c.store(ctx, strconv.Itoa(i), []byte{byte(i)}, time.Millisecond*200); err != nil {
			t.Fatal(err)
		}
	}
	if c.len() != 8 {
		t.Fatal("cache overflow")
	}

	time.Sleep(time.Millisecond * 500)
	if c.len() != 0 {
		t.Fatal()
	}

	if c.cleanerIsRunning != false {
		t.Fatal("cleaner goroutine should be offline")
	}
}
