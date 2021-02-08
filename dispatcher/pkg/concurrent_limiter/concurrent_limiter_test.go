package concurrent_limiter

import (
	"context"
	"sync"
	"testing"
	"time"
)

func Test_ConcurrentLimiter_acquire_release(t *testing.T) {
	l := NewConcurrentLimiter(500)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	wg := new(sync.WaitGroup)
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go func() {
			defer wg.Done()
			select {
			case <-l.Wait():
				time.Sleep(time.Millisecond * 200)
				l.Done()
			case <-ctx.Done():
				t.Fail()
			}
		}()
	}

	wg.Wait()
	if l.Available() != 500 {
		t.Fatal("token leaked")
	}
}
