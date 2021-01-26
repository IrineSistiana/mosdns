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
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"sync"
	"time"
)

// memCache is a simple cache that stores msgs in memory.
type memCache struct {
	size            int
	cleanerInterval time.Duration

	sync.RWMutex
	lru              *utils.LRU
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
		lru:             utils.NewLRU(size, nil),
	}
}

func (c *memCache) get(_ context.Context, key string) (v []byte, ttl time.Duration, ok bool, err error) {
	c.RLock()
	defer c.RUnlock()

	if v, ok := c.lru.Get(key); ok {
		e := v.(*elem)
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
					c.lru.Clean(cleanFunc)
					if c.lru.Len() == 0 {
						c.cleanerIsRunning = false
						c.Unlock()
						return
					}
					c.Unlock()
				}
			}
		}()
	}

	e := &elem{
		v:              v,
		expirationTime: time.Now().Add(ttl),
	}
	c.lru.Add(key, e)
	return
}

func cleanFunc(_ string, v interface{}) bool {
	return v.(*elem).expirationTime.Before(time.Now())
}

func (c *memCache) len() int {
	c.RLock()
	defer c.RUnlock()

	return c.lru.Len()
}
