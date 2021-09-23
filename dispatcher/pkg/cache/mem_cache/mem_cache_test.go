package mem_cache

import (
	"context"
	"github.com/miekg/dns"
	"strconv"
	"sync"
	"testing"
	"time"
)

func Test_memCache(t *testing.T) {
	ctx := context.Background()

	c := NewMemCache(8, 16, -1)
	for i := 0; i < 1024; i++ {
		key := strconv.Itoa(i)
		m := new(dns.Msg)
		m.Id = uint16(i)
		if err := c.Store(ctx, key, m, time.Now(), time.Now().Add(time.Millisecond*200)); err != nil {
			t.Fatal(err)
		}

		v, _, _, err := c.Get(ctx, key, false)
		if err != nil {
			t.Fatal(err)
		}

		if v.Id != uint16(i) {
			t.Fatal("cache kv mismatched")
		}
	}

	if c.len() > 8*16 {
		t.Fatal("cache overflow")
	}
}

func Test_memCache_cleaner(t *testing.T) {
	c := NewMemCache(2, 8, time.Millisecond*10)
	defer c.Close()
	ctx := context.Background()
	for i := 0; i < 64; i++ {
		key := strconv.Itoa(i)
		m := new(dns.Msg)
		m.Id = uint16(i)
		if err := c.Store(ctx, key, m, time.Now(), time.Now().Add(time.Millisecond*10)); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(time.Millisecond * 100)
	if c.len() != 0 {
		t.Fatal()
	}
}

func Test_memCache_race(t *testing.T) {
	c := NewMemCache(32, 128, -1)
	defer c.Close()
	ctx := context.Background()

	m := &dns.Msg{}

	wg := sync.WaitGroup{}
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 256; i++ {
				err := c.Store(ctx, strconv.Itoa(i), m, time.Now(), time.Now().Add(time.Minute))
				if err != nil {
					t.Log(err)
					t.FailNow()
				}
				v, _, _, err := c.Get(ctx, strconv.Itoa(i), false)
				if err != nil {
					t.Log(err)
					t.FailNow()
				}

				v.Id = uint16(i)
				c.lru.Clean(cleanFunc)
			}
		}()
	}
	wg.Wait()
}
