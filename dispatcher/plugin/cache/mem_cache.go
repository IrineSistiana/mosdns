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
	"sync"
	"time"
)

type OnEvictFunc func(key string, v []byte)

// memCache is a simple cache that stores msgs in memory.
type memCache struct {
	size            int
	cleanerInterval time.Duration

	sync.RWMutex
	m                map[string]elem
	cleanerIsRunning bool
}

type elem struct {
	v              []byte
	expirationTime time.Time
}

// newMemCache returns a memCache.
// If cleanerInterval < 0, memCache cleaner is disabled.
// if size <= 0, a default value is used.
// Default size is 1024. Default cleaner interval is 2 minutes.
func newMemCache(size int, cleanerInterval time.Duration) *memCache {
	if size <= 0 {
		size = 1024
	}

	if cleanerInterval == 0 {
		cleanerInterval = time.Minute * 2
	}

	return &memCache{
		size:            size,
		cleanerInterval: cleanerInterval,
		m:               make(map[string]elem, size),
	}
}

func (c *memCache) get(_ context.Context, key string) (v []byte, ttl time.Duration, ok bool, err error) {
	c.RLock()
	defer c.RUnlock()

	if e, ok := c.m[key]; ok {
		if ttl = time.Until(e.expirationTime); ttl > 0 {
			v := make([]byte, len(e.v))
			copy(v, e.v)
			return v, ttl, true, nil
		}
	}
	return nil, 0, false, nil
}

func (c *memCache) store(_ context.Context, key string, v []byte, ttl time.Duration) (err error) {
	if ttl == 0 {
		return
	}

	c.Lock()
	defer c.Unlock()

	// try to start cleaner
	if c.cleanerInterval > 0 && c.cleanerIsRunning == false {
		c.cleanerIsRunning = true

		go func() {
			ticker := time.NewTicker(c.cleanerInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					c.Lock()
					remain, _ := c.clean()
					if remain == 0 {
						c.cleanerIsRunning = false
						c.Unlock()
						return
					}
					c.Unlock()
				}
			}
		}()
	}

	// remove some entries if cache is full.
	n := len(c.m) - c.size + 1
	if n > 0 {
		for key := range c.m {
			if n <= 0 {
				break
			}
			delete(c.m, key)
			n--
		}
	}

	c.m[key] = elem{
		v:              v,
		expirationTime: time.Now().Add(ttl),
	}
	return
}

func (c *memCache) clean() (remain, cleaned int) {
	now := time.Now()
	for key, e := range c.m {
		if e.expirationTime.Before(now) {
			delete(c.m, key)
			cleaned++
		}
	}

	return len(c.m), cleaned
}

func (c *memCache) len() int {
	c.RLock()
	defer c.RUnlock()

	return len(c.m)
}
