package concurrent_map

import (
	"strconv"
	"sync"
	"testing"
)

func TestConcurrentMap(t *testing.T) {
	cm := NewConcurrentMap(8)
	wg := sync.WaitGroup{}

	// test add
	for i := 0; i < 512; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.Set(strconv.Itoa(i), i)
		}()
	}
	wg.Wait()

	// test range
	cc := make([]bool, 512)
	f := func(key string, v interface{}) {
		n := v.(int)
		cc[n] = true
	}
	cm.RangeDo(f)
	for _, ok := range cc {
		if !ok {
			t.Fatal("range failed")
		}
	}

	// test get
	for i := 0; i < 512; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, ok := cm.Get(strconv.Itoa(i))
			if !ok {
				t.Fatal()
			}
			if n, ok := v.(int); !ok || n != i {
				t.Fatal()
			}
		}()
	}
	wg.Wait()

	// test len
	if cm.Len() != 512 {
		t.Fatal()
	}

	// test del
	for i := 0; i < 512; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.Del(strconv.Itoa(i))
		}()
	}
	wg.Wait()
	if cm.Len() != 0 {
		t.Fatal()
	}
}

func TestConcurrentMap_TestAndSet(t *testing.T) {
	cm := NewConcurrentMap(8)
	wg := sync.WaitGroup{}

	f := func(v interface{}, ok bool) (newV interface{}, wantUpdate, passed bool) {
		n := 0
		if ok {
			n = v.(int)
		}
		if n > 0 {
			return nil, false, false
		}
		return 1, true, true
	}

	for i := 0; i < 512; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.TestAndSet("key", f)
		}()
	}
	wg.Wait()

	v, _ := cm.Get("key")
	if v.(int) != 1 {
		t.Fatal()
	}

	// test delete
	f = func(v interface{}, ok bool) (newV interface{}, wantUpdate, passed bool) {
		return nil, true, true
	}
	cm.TestAndSet("key", f)
	_, ok := cm.Get("key")
	if ok {
		t.Fatal()
	}
}

func BenchmarkConcurrentMap_Get_And_Set(b *testing.B) {
	keys := make([]string, 2048)
	m := NewConcurrentMap(64)
	for i := 0; i < 2048; i++ {
		key := strconv.Itoa(i)
		keys[i] = key
		m.Set(key, nil)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(2)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			key := keys[i%2048]

			m.Set(key, nil)
			m.Get(key)
		}
	})
}

func Benchmark_RWMutexMap_Get_And_Set(b *testing.B) {
	keys := make([]string, 2048)
	rwm := new(sync.RWMutex)
	m := make(map[string]interface{}, 2048)
	for i := 0; i < 2048; i++ {
		key := strconv.Itoa(i)
		keys[i] = key
		m[key] = nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.SetParallelism(2)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			key := keys[i%2048]

			rwm.Lock()
			m[key] = nil
			rwm.Unlock()

			rwm.RLock()
			_ = m[key]
			rwm.RUnlock()
		}
	})
}
