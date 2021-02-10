//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package cache

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/concurrent_lru"
	"github.com/miekg/dns"
	"sync"
	"time"
)

type DnsCache interface {
	// Get retrieves v from DnsCache. The returned v is a deepcopy of the original msg
	// that stored in the cache.
	Get(ctx context.Context, key string) (v *dns.Msg, ttl time.Duration, ok bool, err error)
	// Store stores the v into DnsCache. It stores the deepcopy of v.
	Store(ctx context.Context, key string, v *dns.Msg, ttl time.Duration) (err error)

	// Close closes the cache backend.
	Close() error
}

// MemCache is a simple cache that stores msgs in memory.
type MemCache struct {
	cleanerInterval time.Duration

	closeOnce sync.Once
	closeChan chan struct{}
	lru       *concurrent_lru.ConcurrentLRU
}

type elem struct {
	m              *dns.Msg
	expirationTime time.Time
}

// NewMemCache returns a MemCache.
// If cleanerInterval <= 0, MemCache cleaner is disabled.
// If shardNum or maxSizePerShard <=0, NewMemCache will panic.
func NewMemCache(shardNum, maxSizePerShard int, cleanerInterval time.Duration) *MemCache {
	c := &MemCache{
		cleanerInterval: cleanerInterval,
		lru:             concurrent_lru.NewConcurrentLRU(shardNum, maxSizePerShard, nil, nil),
	}

	if c.cleanerInterval > 0 {
		c.closeChan = make(chan struct{})
		go c.startCleaner()
	}
	return c
}

// Close closes the cleaner
func (c *MemCache) Close() error {
	c.closeOnce.Do(func() {
		if c.closeChan != nil {
			close(c.closeChan)
		}
	})
	return nil
}

func (c *MemCache) Get(_ context.Context, key string) (v *dns.Msg, ttl time.Duration, ok bool, err error) {
	e, ok := c.lru.Get(key)

	if ok {
		e := e.(*elem)
		if ttl = time.Until(e.expirationTime); ttl > 0 {
			return e.m.Copy(), ttl, true, nil
		} else {
			c.lru.Del(key) // expired
		}
	}
	return nil, 0, false, nil
}

func (c *MemCache) Store(_ context.Context, key string, v *dns.Msg, ttl time.Duration) (err error) {
	if ttl <= 0 {
		return
	}
	e := &elem{
		m:              v.Copy(),
		expirationTime: time.Now().Add(ttl),
	}
	c.lru.Add(key, e)
	return
}

func (c *MemCache) startCleaner() {
	ticker := time.NewTicker(c.cleanerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.closeChan:
			return
		case <-ticker.C:
			c.lru.Clean(cleanFunc)
		}
	}
}

func cleanFunc(_ string, v interface{}) bool {
	return v.(*elem).expirationTime.Before(time.Now())
}

func (c *MemCache) len() int {
	return c.lru.Len()
}
