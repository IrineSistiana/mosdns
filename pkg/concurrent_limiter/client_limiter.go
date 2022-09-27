/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package concurrent_limiter

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"net/netip"
	"sync"
	"time"
)

type ClientLimiter interface {
	AcquireToken(addr netip.Addr) bool
}

const (
	counterIdleTimeout = time.Second * 10
	hpLimiterShardSize = 64
)

type HPLimiterOpts struct {
	// The rate limit is calculated by Threshold / Interval.
	// Threshold cannot be negative.
	Threshold int
	Interval  time.Duration // Default is 1s.

	// IP masks to aggregate a IP range.
	IPv4Mask int // Default is 32.
	IPv6Mask int // Default is 48.

	// Default is 10s. Negative value disables the cleaner.
	CleanerInterval time.Duration
}

func (opts *HPLimiterOpts) Init() error {
	if opts.Threshold < 0 {
		panic("client_limiter: negative rate")
	}
	utils.SetDefaultNum(&opts.Interval, time.Second)
	utils.SetDefaultNum(&opts.CleanerInterval, time.Second*10)

	if m := opts.IPv4Mask; m < 0 || m > 32 {
		return fmt.Errorf("invalid ipv4 mask %d, should be 0~32", m)
	}

	if m := opts.IPv6Mask; m < 0 || m > 128 {
		return fmt.Errorf("invalid ipv6 mask %d, should be 0~128", m)
	}
	utils.SetDefaultNum(&opts.IPv4Mask, 32)
	utils.SetDefaultNum(&opts.IPv4Mask, 48)
	return nil
}

var _ ClientLimiter = (*HPClientLimiter)(nil)

// HPClientLimiter is a ClientLimiter for heavy workload.
// It uses sharded locks.
type HPClientLimiter struct {
	opts        HPLimiterOpts
	closeOnce   sync.Once
	closeNotify chan struct{}
	shards      [hpLimiterShardSize]*clientLimiter
}

func NewHPClientLimiter(opts HPLimiterOpts) (*HPClientLimiter, error) {
	if err := opts.Init(); err != nil {
		return nil, err
	}
	l := &HPClientLimiter{
		opts:        opts,
		closeNotify: make(chan struct{}),
	}
	for i := range l.shards {
		l.shards[i] = newClientLimiter(opts)
	}

	if opts.CleanerInterval > 0 {
		go l.cleanerLoop()
	}

	return l, nil
}

func (l *HPClientLimiter) cleanerLoop() {
	ticker := time.NewTicker(l.opts.CleanerInterval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			for _, shard := range l.shards {
				shard.clean(now)
			}
		case <-l.closeNotify:
			return
		}
	}
}

func (l *HPClientLimiter) getShard(addr netip.Addr) *clientLimiter {
	n := (addr.As16())[15] % hpLimiterShardSize
	return l.shards[n]
}

func (l *HPClientLimiter) AcquireToken(addr netip.Addr) bool {
	if !addr.IsValid() {
		panic("concurrent_limiter: invalid addr")
	}
	addr = l.ApplyMask(addr)
	return l.getShard(addr).acquireToken(addr)
}

// ApplyMask masks the addr by the mask values in HPLimiterOpts.
func (l *HPClientLimiter) ApplyMask(addr netip.Addr) netip.Addr {
	switch {
	case addr.Is4():
		addr = netip.PrefixFrom(addr, l.opts.IPv4Mask).Masked().Addr()
	case addr.Is4In6():
		addr = netip.PrefixFrom(netip.AddrFrom4(addr.As4()), l.opts.IPv4Mask).Masked().Addr()
	case addr.Is6():
		addr = netip.PrefixFrom(addr, l.opts.IPv6Mask).Masked().Addr()
	}
	return addr
}

// GC removes expired client ip entries from this HPClientLimiter.
func (l *HPClientLimiter) GC(now time.Time) {
	for _, shard := range l.shards {
		shard.clean(now)
	}
}

// Close closes HPClientLimiter's cleaner (if it was started).
// Close always returns a nil error.
func (l *HPClientLimiter) Close() error {
	l.closeOnce.Do(func() {
		close(l.closeNotify)
	})
	return nil
}

// clientLimiter is a ClientLimiter for concurrent use.
// It has an inner lock.
type clientLimiter struct {
	opts HPLimiterOpts

	l sync.Mutex
	m map[netip.Addr]*counter
}

type counter struct {
	c         int
	startTime time.Time
}

// newClientLimiter returns a new clientLimiter.
// The opts must be initialized.
func newClientLimiter(opts HPLimiterOpts) *clientLimiter {
	l := &clientLimiter{
		opts: opts,
		m:    make(map[netip.Addr]*counter),
	}
	return l
}

func (l *clientLimiter) acquireToken(addr netip.Addr) bool {
	now := time.Now()

	l.l.Lock()
	defer l.l.Unlock()

	e, ok := l.m[addr]
	if !ok {
		e = new(counter)
		l.m[addr] = e
	}

	// Another interval is passed. Reset the counter.
	if e.startTime.Add(l.opts.Interval).Before(now) {
		e.startTime = now
		e.c = 0
	}

	if e.c < l.opts.Threshold {
		e.c++
		return true
	}
	return false
}

func (l *clientLimiter) clean(now time.Time) {
	l.l.Lock()
	defer l.l.Unlock()

	for addr, counter := range l.m {
		if counter.startTime.Add(counterIdleTimeout).Before(now) {
			delete(l.m, addr)
		}
	}
}
