//     Copyright (C) 2020, IrineSistiana
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
	"github.com/miekg/dns"
	"sync"
	"time"
)

// cache is a simple dns cache. It won't cache err msg (rcode != 0).
type cache struct {
	size            int
	cleanerInterval time.Duration

	sync.RWMutex
	m                map[string]elem
	cleanerIsRunning bool
}

type elem struct {
	v              *dns.Msg
	expirationTime time.Time
}

// newCache returns a cache object.
// If cleanerInterval < 0, cache cleaner is disabled.
// if size <= 0, a default value is used.
// Default size is 1024. Default cleaner interval is 10 sec.
func newCache(size int, cleanerInterval time.Duration) *cache {
	if size <= 0 {
		size = 1024
	}

	if cleanerInterval == 0 {
		cleanerInterval = time.Second * 10
	}

	return &cache{
		size:            size,
		cleanerInterval: cleanerInterval,
		m:               make(map[string]elem, size),
	}
}

func (c *cache) add(key string, ttl uint32, r *dns.Msg) {
	if ttl == 0 || r == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	// try to start cleaner
	if c.cleanerInterval > 0 && c.cleanerIsRunning == false {
		c.cleanerIsRunning = true

		go func() {
			defer func() {
				c.Lock()
				defer c.Unlock()
				c.cleanerIsRunning = false
			}()

			ticker := time.NewTicker(c.cleanerInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					remain, _ := c.clean()
					if remain == 0 {
						return
					}
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
		v:              r,
		expirationTime: time.Now().Add(time.Duration(ttl) * time.Second),
	}

	return
}

func (c *cache) get(key string) (r *dns.Msg, ttl time.Duration) {
	c.RLock()
	defer c.RUnlock()

	if e, ok := c.m[key]; ok {
		if ttl = time.Until(e.expirationTime); ttl > 0 {
			m := new(dns.Msg)
			e.v.CopyTo(m)
			return m, ttl
		}
	}
	return nil, 0
}

func (c *cache) clean() (remain, cleaned int) {
	c.Lock()
	defer c.Unlock()

	now := time.Now()
	for key, e := range c.m {
		if e.expirationTime.Before(now) {
			delete(c.m, key)
			cleaned++
		}
	}

	return len(c.m), cleaned
}

func (c *cache) len() int {
	c.RLock()
	defer c.RUnlock()

	return len(c.m)
}

func getMinimalTTL(m *dns.Msg) uint32 {
	if m == nil || len(m.Answer)+len(m.Ns)+len(m.Extra) == 0 {
		return 0
	}

	ttl := ^uint32(0)
	for _, r := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for i := range r {
			t := r[i].Header().Ttl
			if t < ttl {
				ttl = t
			}
		}
	}

	return ttl
}

func setTTL(m *dns.Msg, ttl uint32) {
	if m == nil || len(m.Answer)+len(m.Ns)+len(m.Extra) == 0 {
		return
	}

	for _, r := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for i := range r {
			r[i].Header().Ttl = ttl
		}
	}
}
