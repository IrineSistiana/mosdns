package mem_cache

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"
)

func Test_memCache(t *testing.T) {
	ctx := context.Background()

	c := NewMemCache(1024, 0)
	for i := 0; i < 128; i++ {
		key := strconv.Itoa(i)
		if err := c.Store(ctx, key, []byte{byte(i)}, time.Now(), time.Now().Add(time.Millisecond*200)); err != nil {
			t.Fatal(err)
		}

		v, _, _, err := c.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}

		if v[0] != byte(i) {
			t.Fatal("cache kv mismatched")
		}
	}

	for i := 0; i < 1024*4; i++ {
		key := strconv.Itoa(i)
		if err := c.Store(ctx, key, []byte{}, time.Now(), time.Now().Add(time.Millisecond*200)); err != nil {
			t.Fatal(err)
		}
	}

	if c.Len() > 1024 {
		t.Fatal("cache overflow")
	}
}

func Test_memCache_cleaner(t *testing.T) {
	c := NewMemCache(1024, time.Millisecond*10)
	defer c.Close()
	ctx := context.Background()
	for i := 0; i < 64; i++ {
		key := strconv.Itoa(i)
		if err := c.Store(ctx, key, make([]byte, 0), time.Now(), time.Now().Add(time.Millisecond*10)); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(time.Millisecond * 100)
	if c.Len() != 0 {
		t.Fatal()
	}
}

func Test_memCache_race(t *testing.T) {
	c := NewMemCache(1024, -1)
	defer c.Close()
	ctx := context.Background()

	wg := sync.WaitGroup{}
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 256; i++ {
				err := c.Store(ctx, strconv.Itoa(i), []byte{}, time.Now(), time.Now().Add(time.Minute))
				if err != nil {
					t.Log(err)
					t.FailNow()
				}
				_, _, _, err = c.Get(ctx, strconv.Itoa(i))
				if err != nil {
					t.Log(err)
					t.FailNow()
				}
				c.lru.Clean(c.cleanFunc())
			}
		}()
	}
	wg.Wait()
}
