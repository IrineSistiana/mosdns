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

package mem_cache

import (
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/concurrent_lru"
	"sync/atomic"
	"time"
)

const (
	shardSize              = 64
	defaultCleanerInterval = time.Minute
)

// MemCache is a simple LRU cache that stores values in memory.
// It is safe for concurrent use.
type MemCache struct {
	closed           uint32
	closeCleanerChan chan struct{}
	lru              *concurrent_lru.ShardedLRU[*elem]
}

type elem struct {
	v              []byte
	storedTime     time.Time
	expirationTime time.Time
}

// NewMemCache initializes a MemCache.
// The minimum size is 1024.
// cleanerInterval specifies the interval that MemCache scans
// and discards expired values. If cleanerInterval <= 0, a default
// interval will be used.
func NewMemCache(size int, cleanerInterval time.Duration) *MemCache {
	sizePerShard := size / shardSize
	if sizePerShard < 16 {
		sizePerShard = 16
	}

	c := &MemCache{
		closeCleanerChan: make(chan struct{}),
		lru:              concurrent_lru.NewShardedLRU[*elem](shardSize, sizePerShard, nil),
	}
	go c.startCleaner(cleanerInterval)
	return c
}

func (c *MemCache) isClosed() bool {
	return atomic.LoadUint32(&c.closed) != 0
}

// Close closes the cache and its cleaner.
func (c *MemCache) Close() error {
	if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
		close(c.closeCleanerChan)
	}
	return nil
}

func (c *MemCache) Get(key string) (v []byte, storedTime, expirationTime time.Time) {
	if c.isClosed() {
		return nil, time.Time{}, time.Time{}
	}

	if e, ok := c.lru.Get(key); ok {
		return e.v, e.storedTime, e.expirationTime
	}

	// no such key
	return nil, time.Time{}, time.Time{}
}

func (c *MemCache) Store(key string, v []byte, storedTime, expirationTime time.Time) {
	if c.isClosed() {
		return
	}

	now := time.Now()
	if now.After(expirationTime) {
		return
	}

	buf := make([]byte, len(v))
	copy(buf, v)

	e := &elem{
		v:              buf,
		storedTime:     storedTime,
		expirationTime: expirationTime,
	}
	c.lru.Add(key, e)
	return
}

func (c *MemCache) startCleaner(interval time.Duration) {
	if interval <= 0 {
		interval = defaultCleanerInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.closeCleanerChan:
			return
		case <-ticker.C:
			c.lru.Clean(c.cleanFunc())
		}
	}
}

func (c *MemCache) cleanFunc() func(_ string, v *elem) bool {
	now := time.Now()
	return func(_ string, v *elem) bool {
		return v.expirationTime.Before(now)
	}
}

func (c *MemCache) Len() int {
	return c.lru.Len()
}
