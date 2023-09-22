package rate_limiter

import (
	"io"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter interface {
	Allow(addr netip.Addr) bool
	io.Closer
}

type limiter struct {
	limit rate.Limit
	burst int
	mask4 int
	mask6 int

	closeOnce   sync.Once
	closeNotify chan struct{}
	m           sync.Mutex
	tables      map[netip.Addr]*limiterEntry
}

type limiterEntry struct {
	l        *rate.Limiter
	lastSeen time.Time
	sync.Once
}

// limit and burst should be greater than zero.
// If gcInterval is <= 0, it will be automatically chosen between 2~10s.
// In this case, if the token refill time (burst/limit) is greater than 10s,
// the actual average qps limit may be higher than expected.
// If mask is zero or greater than 32/128. The default is 32/48.
// If mask is negative, the masks will be 0.
func NewRateLimiter(limit rate.Limit, burst int, gcInterval time.Duration, mask4, mask6 int) RateLimiter {
	if mask4 > 32 || mask4 == 0 {
		mask4 = 32
	}
	if mask4 < 0 {
		mask4 = 0
	}

	if mask6 > 128 || mask6 == 0 {
		mask6 = 48
	}
	if mask6 < 0 {
		mask6 = 0
	}

	if gcInterval <= 0 {
		if limit <= 0 || burst <= 0 {
			gcInterval = time.Second * 2
		} else {
			refillSec := float64(burst) / float64(limit)
			if refillSec < 2 {
				refillSec = 2
			}
			if refillSec > 10 {
				refillSec = 10
			}
			gcInterval = time.Duration(refillSec) * time.Second
		}
	}

	l := &limiter{
		limit:       limit,
		burst:       burst,
		mask4:       mask4,
		mask6:       mask6,
		closeNotify: make(chan struct{}),
		tables:      make(map[netip.Addr]*limiterEntry),
	}
	go l.gcLoop(gcInterval)
	return l
}

func (l *limiter) Allow(a netip.Addr) bool {
	a = l.applyMask(a)
	now := time.Now()
	l.m.Lock()
	e, ok := l.tables[a]
	if !ok {
		e = &limiterEntry{
			l:        rate.NewLimiter(l.limit, l.burst),
			lastSeen: now,
		}
		l.tables[a] = e
	}
	e.lastSeen = now
	clientLimiter := e.l
	l.m.Unlock()
	return clientLimiter.AllowN(now, 1)
}

func (l *limiter) Close() error {
	l.closeOnce.Do(func() {
		close(l.closeNotify)
	})
	return nil
}

func (l *limiter) gcLoop(gcInterval time.Duration) {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.closeNotify:
			return
		case now := <-ticker.C:
			l.doGc(now, gcInterval)
		}
	}
}

func (l *limiter) doGc(now time.Time, gcInterval time.Duration) {
	l.m.Lock()
	defer l.m.Unlock()

	for a, e := range l.tables {
		if now.Sub(e.lastSeen) > gcInterval {
			delete(l.tables, a)
		}
	}
}

func (l *limiter) applyMask(a netip.Addr) netip.Addr {
	switch {
	case a.Is4():
		m, _ := a.Prefix(l.mask4)
		return m.Addr()
	case a.Is4In6():
		m, _ := netip.AddrFrom4(a.As4()).Prefix(l.mask4)
		return m.Addr()
	default:
		m, _ := a.Prefix(l.mask6)
		return m.Addr()
	}
}
