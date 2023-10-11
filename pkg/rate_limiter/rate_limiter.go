package rate_limiter

import (
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	tableShards = 32
	gcInterval  = time.Minute
)

type Limiter struct {
	// Limit and Burst are read-only.
	Limit rate.Limit
	Burst int

	closeOnce   sync.Once
	closeNotify chan struct{}
	tables      [tableShards]*tableShard
}

type tableShard struct {
	m     sync.Mutex
	table map[netip.Addr]*limiterEntry
}

type limiterEntry struct {
	l        *rate.Limiter
	lastSeen time.Time
	sync.Once
}

// NewRateLimiter creates a new client rate limiter.
// limit and burst should be greater than zero. See rate.Limiter for more
// details.
// Limiter has a internal gc which will run and remove old client entries every 1m.
// If the token refill time (burst/limit) is greater than 1m,
// the actual average qps limit may be higher than expected because the client status
// may be deleted and re-initialized.
func NewRateLimiter(limit rate.Limit, burst int) *Limiter {
	l := &Limiter{
		Limit:       limit,
		Burst:       burst,
		closeNotify: make(chan struct{}),
	}

	for i := range l.tables {
		l.tables[i] = &tableShard{table: make(map[netip.Addr]*limiterEntry)}
	}

	go l.gcLoop(gcInterval)
	return l
}

// maskedUnmappedP must be a masked prefix and contain a unmapped addr.
func (l *Limiter) Allow(unmappedAddr netip.Addr) bool {
	now := time.Now()
	shard := l.getTableShard(unmappedAddr)
	shard.m.Lock()
	e, ok := shard.table[unmappedAddr]
	if !ok {
		e = &limiterEntry{
			l:        rate.NewLimiter(l.Limit, l.Burst),
			lastSeen: now,
		}
		shard.table[unmappedAddr] = e
	}
	e.lastSeen = now
	shard.m.Unlock()
	clientLimiter := e.l
	return clientLimiter.AllowN(now, 1)
}

func (l *Limiter) Close() error {
	l.closeOnce.Do(func() {
		close(l.closeNotify)
	})
	return nil
}

func (l *Limiter) gcLoop(gcInterval time.Duration) {
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

func (l *Limiter) doGc(now time.Time, gcInterval time.Duration) {
	for _, shard := range l.tables {
		shard.m.Lock()
		for a, e := range shard.table {
			if now.Sub(e.lastSeen) > gcInterval {
				delete(shard.table, a)
			}
		}
		shard.m.Unlock()
	}
}

func (l *Limiter) getTableShard(unmappedAddr netip.Addr) *tableShard {
	return l.tables[getTableShardIdx(unmappedAddr)]
}

func (l *Limiter) ForEach(doFunc func(unmappedAddr netip.Addr, r *rate.Limiter) (doBreak bool)) (doBreak bool) {
	for _, shard := range l.tables {
		shard.m.Lock()
		for a, e := range shard.table {
			doBreak = doFunc(a, e.l)
			if doBreak {
				shard.m.Unlock()
				return
			}
		}
		shard.m.Unlock()
	}
	return false
}

// Len returns current number of entries in the Limiter.
func (l *Limiter) Len() int {
	n := 0
	for _, shard := range l.tables {
		shard.m.Lock()
		n += len(shard.table)
		shard.m.Unlock()
	}
	return n
}

func getTableShardIdx(unmappedAddr netip.Addr) int {
	var i byte
	if unmappedAddr.Is4() {
		for _, b := range unmappedAddr.As4() {
			i ^= b
		}
	} else {
		for _, b := range unmappedAddr.As16() {
			i ^= b
		}
	}
	return int(i % tableShards)
}
