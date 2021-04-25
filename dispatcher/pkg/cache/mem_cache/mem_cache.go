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
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/concurrent_lru"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
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

func (c *MemCache) Get(_ context.Context, key string) (*dns.Msg, error) {
	e, ok := c.lru.Get(key)

	if ok {
		e := e.(*elem)
		if time.Until(e.expirationTime) > 0 {
			deltaTTL := uint32(time.Since(e.storedTime) / time.Second)
			v := new(dns.Msg)
			if err := v.Unpack(e.m); err != nil {
				return nil, err
			}
			dnsutils.SubtractTTL(v, deltaTTL)
			return v, nil
		} else {
			c.lru.Del(key) // expired
		}
	}
	return nil, nil
}

func (c *MemCache) Store(_ context.Context, key string, v *dns.Msg, ttl time.Duration) (err error) {
	if ttl <= 0 {
		return
	}

	if v == nil {
		return errors.New("nil v")
	}

	b, err := v.Pack()
	if err != nil {
		return err
	}

	now := time.Now()
	e := &elem{
		m:              b,
		storedTime:     now,
		expirationTime: now.Add(ttl),
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
