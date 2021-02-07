package utils

import (
	"strconv"
	"testing"
)

func TestNewClientQueryLimiter(t *testing.T) {
	limiter := NewClientQueryLimiter(8)

	key := "key"
	for i := 0; i < 16; i++ {
		ok := limiter.Acquire(key)

		if i < 8 && !ok { // if it not reaches the limit but return a false
			t.Fatal()
		}

		if i >= 8 && ok { // if it reached the limit but return a true
			t.Fatal()
		}
	}

	for i := 0; i < 8; i++ {
		limiter.Done(key)
	}

	func() {
		defer func() {
			msg := recover()
			if msg == nil {
				t.Fatal("invalid Done call should panic")
			}
		}()
		limiter.Done(key)
	}()

	func() {
		defer func() {
			msg := recover()
			if msg == nil {
				t.Fatal("Done should panic when key is not exist")
			}
		}()
		limiter.Done(key + " ")
	}()

	if limiter.m.Len() != 0 {
		t.Fatal()
	}
}

func TestNewClientQueryLimiter_race(t *testing.T) {
	limiter := NewClientQueryLimiter(8)
	for i := 0; i < 512; i++ {
		key := strconv.Itoa(i)
		for k := 0; k < 8; k++ {
			go func() {
				if !limiter.Acquire(key) {
					t.Fail()
				}
				limiter.Done(key)
			}()
		}
	}
}
