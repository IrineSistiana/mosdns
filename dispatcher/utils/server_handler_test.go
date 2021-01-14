package utils

import (
	"context"
	"sync"
	"testing"
	"time"
)

func Test_newConcurrentLimiter(t *testing.T) {
	type args struct {
		max int
	}
	tests := []struct {
		name    string
		args    args
		wantLen int
	}{
		{"1", args{max: 1}, 1},
		{"2", args{max: 50}, 50},
		{"3", args{max: 1000}, 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newConcurrentLimiter(tt.args.max); got.available() != tt.wantLen {
				t.Errorf("newConcurrentLimiter() = %v, want %v", got.available(), tt.wantLen)
			}
		})
	}
}

func Test_concurrentLimiter_acquire_release(t *testing.T) {
	l := newConcurrentLimiter(500)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	wg := new(sync.WaitGroup)
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go func() {
			defer wg.Done()
			select {
			case <-l.acquire():
				time.Sleep(time.Millisecond * 200)
				l.release()
			case <-ctx.Done():
				t.Fail()
			}
		}()
	}

	wg.Wait()
	if l.available() != 500 {
		t.Fatal("token leak")
	}
}
