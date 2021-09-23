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

package mem_cache

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/concurrent_lru"
	"github.com/miekg/dns"
	"sync"
	"time"
)

// MemCache is a simple cache that stores msgs in memory.
type MemCache struct {
	cleanerInterval time.Duration

	closeCleanerOnce sync.Once
	closeCleanerChan chan struct{}
	lru              *concurrent_lru.ConcurrentLRU
}

type elem struct {
	m              []byte
	storedTime     time.Time
	expirationTime time.Time
}

// NewMemCache returns a MemCache.
// If cleanerInterval <= 0, MemCache cleaner is disabled.
func NewMemCache(shardNum, maxSizePerShard int, cleanerInterval time.Duration) *MemCache {
	c := &MemCache{
		cleanerInterval: cleanerInterval,
		lru:             concurrent_lru.NewConcurrentLRU(shardNum, maxSizePerShard, nil, nil),
	}

	if c.cleanerInterval > 0 {
		c.closeCleanerChan = make(chan struct{})
		go c.startCleaner()
	}
	return c
}

// Close closes the cleaner
func (c *MemCache) Close() error {
	c.closeCleanerOnce.Do(func() {
		if c.closeCleanerChan != nil {
			close(c.closeCleanerChan)
		}
	})
	return nil
}

func (c *MemCache) Get(_ context.Context, key string, allowExpired bool) (m *dns.Msg, storedTime, expirationTime time.Time, err error) {
	if v, ok := c.lru.Get(key); ok {
		e := v.(*elem)
		if !allowExpired && e.expirationTime.Before(time.Now()) { // suppress expired result
			return nil, time.Time{}, time.Time{}, nil
		}

		m := new(dns.Msg)
		if err := m.Unpack(e.m); err != nil {
			return nil, time.Time{}, time.Time{}, err
		}
		return m, e.storedTime, e.expirationTime, nil
	}

	// no such key
	return nil, time.Time{}, time.Time{}, nil
}

func (c *MemCache) Store(_ context.Context, key string, v *dns.Msg, storedTime, expirationTime time.Time) (err error) {
	now := time.Now()
	if now.After(expirationTime) {
		return
	}

	b, err := v.Pack()
	if err != nil {
		return err
	}

	e := &elem{
		m:              b,
		storedTime:     storedTime,
		expirationTime: expirationTime,
	}
	c.lru.Add(key, e)
	return
}

func (c *MemCache) startCleaner() {
	ticker := time.NewTicker(c.cleanerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.closeCleanerChan:
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
