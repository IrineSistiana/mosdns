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

package cache

import (
	"github.com/IrineSistiana/mosdns/v5/pkg/concurrent_lru"
	"github.com/IrineSistiana/mosdns/v5/pkg/concurrent_map"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"sync"
	"time"
)

const (
	defaultCleanerInterval = time.Second * 10
)

type Key interface {
	concurrent_lru.Hashable
}

type Value interface {
	any
}

// Cache is a simple LRU cache that stores values in memory.
// It is safe for concurrent use.
type Cache[K Key, V Value] struct {
	opts Opts

	closeOnce   sync.Once
	closeNotify chan struct{}
	m           *concurrent_map.Map[K, *elem[V]]
}

type Opts struct {
	Size            int
	CleanerInterval time.Duration
}

func (opts *Opts) init() {
	utils.SetDefaultNum(&opts.Size, 1024)
	utils.SetDefaultNum(&opts.CleanerInterval, defaultCleanerInterval)
}

type elem[V Value] struct {
	v              V
	storedTime     time.Time
	expirationTime time.Time
}

// New initializes a Cache.
// The minimum size is 1024.
// cleanerInterval specifies the interval that Cache scans
// and discards expired values. If cleanerInterval <= 0, a default
// interval will be used.
func New[K Key, V Value](opts Opts) *Cache[K, V] {
	opts.init()
	c := &Cache[K, V]{
		closeNotify: make(chan struct{}),
		m:           concurrent_map.NewMapCache[K, *elem[V]](opts.Size),
	}
	go c.gcLoop(opts.CleanerInterval)
	return c
}

func (c *Cache[K, V]) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeNotify)
	})
	return nil
}

func (c *Cache[K, V]) Get(key K) (v V, storedTime, expirationTime time.Time, ok bool) {
	if e, hasEntry := c.m.Get(key); hasEntry {
		if e.expirationTime.Before(time.Now()) {
			c.m.Del(key)
			return
		}
		return e.v, e.storedTime, e.expirationTime, true
	}
	return
}

func (c *Cache[K, V]) Range(f func(key K, v V, storedTime, expirationTime time.Time)) {
	cf := func(key K, v *elem[V]) (newV *elem[V], setV bool, delV bool) {
		f(key, v.v, v.storedTime, v.expirationTime)
		return
	}
	c.m.RangeDo(cf)
}

func (c *Cache[K, V]) Store(key K, v V, storedTime, expirationTime time.Time) {
	now := time.Now()
	if now.After(expirationTime) {
		return
	}
	if storedTime.IsZero() {
		storedTime = now
	}

	e := &elem[V]{
		v:              v,
		storedTime:     storedTime,
		expirationTime: expirationTime,
	}
	c.m.Set(key, e)
	return
}

func (c *Cache[K, V]) gcLoop(interval time.Duration) {
	if interval <= 0 {
		interval = defaultCleanerInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.closeNotify:
			return
		case now := <-ticker.C:
			c.gc(now)
		}
	}
}

func (c *Cache[K, V]) gc(now time.Time) {
	f := func(key K, v *elem[V]) (newV *elem[V], setV bool, delV bool) {
		return nil, false, now.After(v.expirationTime)
	}
	c.m.RangeDo(f)
}

func (c *Cache[K, V]) Len() int {
	return c.m.Len()
}

func (c *Cache[K, V]) Flush() {
	c.m.Flush()
}
