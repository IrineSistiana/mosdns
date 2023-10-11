package rate_limiter

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func BenchmarkXxx(b *testing.B) {
	now := time.Now()
	var l *limiterEntry
	for i := 0; i < b.N; i++ {
		l = &limiterEntry{
			l:        rate.NewLimiter(0, 0),
			lastSeen: now,
		}
	}
	_ = l
}
