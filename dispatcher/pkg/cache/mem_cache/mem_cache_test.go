package mem_cache

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func Test_memCache(t *testing.T) {
	c := NewMemCache(1024, 0)
	for i := 0; i < 128; i++ {
		key := strconv.Itoa(i)
		c.Store(key, []byte{byte(i)}, time.Now(), time.Now().Add(time.Millisecond*200))
		v, _, _ := c.Get(key)

		if v[0] != byte(i) {
			t.Fatal("cache kv mismatched")
		}
	}

	for i := 0; i < 1024*4; i++ {
		key := strconv.Itoa(i)
		c.Store(key, []byte{}, time.Now(), time.Now().Add(time.Millisecond*200))
	}

	if c.Len() > 1024 {
		t.Fatal("cache overflow")
	}
}

func Test_memCache_cleaner(t *testing.T) {
	c := NewMemCache(1024, time.Millisecond*10)
	defer c.Close()
	for i := 0; i < 64; i++ {
		key := strconv.Itoa(i)
		c.Store(key, make([]byte, 0), time.Now(), time.Now().Add(time.Millisecond*10))
	}

	time.Sleep(time.Millisecond * 100)
	if c.Len() != 0 {
		t.Fatal()
	}
}

func Test_memCache_race(t *testing.T) {
	c := NewMemCache(1024, -1)
	defer c.Close()

	wg := sync.WaitGroup{}
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 256; i++ {
				c.Store(strconv.Itoa(i), []byte{}, time.Now(), time.Now().Add(time.Minute))
				_, _, _ = c.Get(strconv.Itoa(i))
				c.lru.Clean(c.cleanFunc())
			}
		}()
	}
	wg.Wait()
}
