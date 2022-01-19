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

package concurrent_map

import (
	"hash/maphash"
	"sync"
)

// ConcurrentMap is a map that is safe for concurrent use.
// It usually has better performance than (sync.RWMutex + map) under high workload.
type ConcurrentMap struct {
	seed maphash.Seed
	m    []*shardedMap
}

type TestAndSetFunc func(v interface{}, ok bool) (newV interface{}, wantUpdate, testPassed bool)

func NewConcurrentMap(shard int) *ConcurrentMap {
	cm := &ConcurrentMap{m: make([]*shardedMap, shard), seed: maphash.MakeSeed()}
	for i := range cm.m {
		cm.m[i] = &shardedMap{
			m: make(map[string]interface{}),
		}
	}
	return cm
}

func (c *ConcurrentMap) Set(key string, v interface{}) {
	sm := c.getShardedMap(key)
	sm.set(key, v)
}

// TestAndSet is a concurrent safe test-and-set operation.
// If f returns nil, the key will be deleted.
func (c *ConcurrentMap) TestAndSet(key string, f TestAndSetFunc) (passed bool) {
	sm := c.getShardedMap(key)
	return sm.testAndSet(key, f)
}

func (c *ConcurrentMap) Get(key string) (v interface{}, ok bool) {
	sm := c.getShardedMap(key)
	v, ok = sm.get(key)
	return
}

func (c *ConcurrentMap) Del(key string) {
	sm := c.getShardedMap(key)
	sm.del(key)
}

func (c *ConcurrentMap) Len() int {
	sum := 0
	for i := range c.m {
		sum += c.m[i].len()
	}
	return sum
}

func (c *ConcurrentMap) RangeDo(f func(key string, v interface{})) {
	for i := range c.m {
		c.m[i].rangeDo(f)
	}
}

func (c *ConcurrentMap) shardNum() int {
	return len(c.m)
}

func (c *ConcurrentMap) getShardedMap(key string) *shardedMap {
	h := maphash.Hash{}
	h.SetSeed(c.seed)

	h.WriteString(key)
	n := h.Sum64() % uint64(c.shardNum())
	return c.m[n]
}

type shardedMap struct {
	sync.RWMutex
	m map[string]interface{}
}

func (sm *shardedMap) set(key string, v interface{}) {
	sm.Lock()
	defer sm.Unlock()

	sm.m[key] = v
}

func (sm *shardedMap) testAndSet(key string, f TestAndSetFunc) (testPassed bool) {
	sm.Lock()
	defer sm.Unlock()

	v, ok := sm.m[key]
	newV, wantUpdate, passed := f(v, ok)
	if wantUpdate {
		if newV != nil {
			sm.m[key] = newV
		} else {
			if ok {
				delete(sm.m, key)
			}
		}
	}

	return passed
}

func (sm *shardedMap) get(key string) (v interface{}, ok bool) {
	sm.RLock()
	defer sm.RUnlock()

	v, ok = sm.m[key]
	return
}

func (sm *shardedMap) rangeDo(f func(key string, v interface{})) {
	sm.RLock()
	defer sm.RUnlock()
	for key, v := range sm.m {
		f(key, v)
	}
}

func (sm *shardedMap) del(key string) {
	sm.Lock()
	defer sm.Unlock()

	delete(sm.m, key)
}

func (sm *shardedMap) len() int {
	sm.RLock()
	defer sm.RUnlock()

	return len(sm.m)
}
