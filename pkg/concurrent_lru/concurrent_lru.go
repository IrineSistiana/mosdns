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

package concurrent_lru

import (
	"github.com/IrineSistiana/mosdns/v4/pkg/lru"
	"hash/maphash"
	"sync"
)

type ConcurrentLRU struct {
	seed maphash.Seed
	l    []*shardedLRU
}

func NewConcurrentLRU(
	shardNum, maxSizePerShard int,
	onEvict func(key string, v interface{}),
	onGet func(key string, v interface{}) interface{},
) *ConcurrentLRU {
	cl := &ConcurrentLRU{
		seed: maphash.MakeSeed(),
		l:    make([]*shardedLRU, shardNum),
	}

	for i := range cl.l {
		cl.l[i] = &shardedLRU{
			onGet: onGet,
			lru:   lru.NewLRU(maxSizePerShard, onEvict),
		}
	}

	return cl
}

func (c *ConcurrentLRU) Add(key string, v interface{}) {
	sl := c.getShardedLRU(key)
	sl.Add(key, v)
}

func (c *ConcurrentLRU) Del(key string) {
	sl := c.getShardedLRU(key)
	sl.Del(key)
}

func (c *ConcurrentLRU) Clean(f func(key string, v interface{}) (remove bool)) (removed int) {
	for i := range c.l {
		removed += c.l[i].Clean(f)
	}
	return removed
}

func (c *ConcurrentLRU) Get(key string) (v interface{}, ok bool) {
	sl := c.getShardedLRU(key)
	v, ok = sl.Get(key)
	return
}

func (c *ConcurrentLRU) Len() int {
	sum := 0
	for _, lru := range c.l {
		sum += lru.Len()
	}
	return sum
}

func (c *ConcurrentLRU) shardNum() int {
	return len(c.l)
}

func (c *ConcurrentLRU) getShardedLRU(key string) *shardedLRU {
	h := maphash.Hash{}
	h.SetSeed(c.seed)

	h.WriteString(key)
	n := h.Sum64() % uint64(c.shardNum())
	return c.l[n]
}

type shardedLRU struct {
	onGet func(key string, v interface{}) interface{}

	sync.Mutex
	lru *lru.LRU
}

func (sl *shardedLRU) Add(key string, v interface{}) {
	sl.Lock()
	defer sl.Unlock()

	sl.lru.Add(key, v)
}

func (sl *shardedLRU) Del(key string) {
	sl.Lock()
	defer sl.Unlock()

	sl.lru.Del(key)
}

func (sl *shardedLRU) Clean(f func(key string, v interface{}) (remove bool)) (removed int) {
	sl.Lock()
	defer sl.Unlock()

	return sl.lru.Clean(f)
}

func (sl *shardedLRU) Get(key string) (v interface{}, ok bool) {
	sl.Lock()
	defer sl.Unlock()

	v, ok = sl.lru.Get(key)
	if ok && sl.onGet != nil {
		v = sl.onGet(key, v)
	}
	return
}

func (sl *shardedLRU) Len() int {
	sl.Lock()
	defer sl.Unlock()

	return sl.lru.Len()
}
