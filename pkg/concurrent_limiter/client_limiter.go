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
	"net/netip"
	"sync"
	"time"
)

type ClientLimiter interface {
	Acquire(addr netip.Addr) bool
	GC(now time.Time)
}

const (
	counterIdleTimeout = time.Second * 10
	hpLimiterShardSize = 64
)

var _ ClientLimiter = (*ClientLimiterNoLock)(nil)

// ClientLimiterNoLock is a simple ClientLimiter for single thread.
type ClientLimiterNoLock struct {
	maxQPS int
	m      map[netip.Addr]*counter
}

type counter struct {
	c         int
	startTime time.Time
}

func NewClientLimiterNoLock(maxQPS int) ClientLimiterNoLock {
	return ClientLimiterNoLock{
		maxQPS: maxQPS,
		m:      make(map[netip.Addr]*counter),
	}
}

func (l *ClientLimiterNoLock) Acquire(addr netip.Addr) bool {
	now := time.Now()
	e, ok := l.m[addr]
	if !ok {
		e = new(counter)
		l.m[addr] = e
	}

	// Another second is passed. Reset the counter.
	if e.startTime.Add(time.Second).Before(now) {
		e.startTime = now
		e.c = 0
	}

	if e.c <= l.maxQPS {
		e.c++
		return true
	}
	return false
}

func (l *ClientLimiterNoLock) GC(now time.Time) {
	for addr, counter := range l.m {
		if counter.startTime.Add(counterIdleTimeout).Before(now) {
			delete(l.m, addr)
		}
	}
}

// ClientLimiterWithLock is a simple ClientLimiter. It has a inner lock.
// So it's safe for concurrent use.
type ClientLimiterWithLock struct {
	l      sync.Mutex
	noLock ClientLimiterNoLock
}

func NewClientLimiterWithLock(maxQPS int) *ClientLimiterWithLock {
	return &ClientLimiterWithLock{
		noLock: NewClientLimiterNoLock(maxQPS),
	}
}

func (l *ClientLimiterWithLock) Acquire(addr netip.Addr) bool {
	l.l.Lock()
	defer l.l.Unlock()
	return l.noLock.Acquire(addr)
}

func (l *ClientLimiterWithLock) GC(now time.Time) {
	l.l.Lock()
	defer l.l.Unlock()
	l.noLock.GC(now)
}

// HPClientLimiter is a ClientLimiter for heavy work load.
// It uses sharded locks.
type HPClientLimiter struct {
	shards [hpLimiterShardSize]*ClientLimiterWithLock
}

func NewHPClientLimiter(maxQPS int) *HPClientLimiter {
	l := &HPClientLimiter{}
	for i := range l.shards {
		l.shards[i] = NewClientLimiterWithLock(maxQPS)
	}
	return l
}

func (l *HPClientLimiter) Acquire(addr netip.Addr) bool {
	shard := l.getShard(addr)
	return shard.Acquire(addr)
}

func (l *HPClientLimiter) getShard(addr netip.Addr) *ClientLimiterWithLock {
	n := (addr.As16())[15] % hpLimiterShardSize
	return l.shards[n]
}

func (l *HPClientLimiter) GC(now time.Time) {
	for _, shard := range l.shards {
		shard.GC(now)
	}
}
