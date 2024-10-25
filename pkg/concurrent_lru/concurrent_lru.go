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

type ShardedLRU[V any] struct {
	seed maphash.Seed
	l    []*ConcurrentLRU[string, V]
}

func NewShardedLRU[V any](
	shardNum, maxSizePerShard int,
	onEvict func(key string, v V),
) *ShardedLRU[V] {
	cl := &ShardedLRU[V]{
		seed: maphash.MakeSeed(),
		l:    make([]*ConcurrentLRU[string, V], shardNum),
	}

	for i := range cl.l {
		cl.l[i] = &ConcurrentLRU[string, V]{
			lru: lru.NewLRU[string, V](maxSizePerShard, onEvict),
		}
	}

	return cl
}

func (c *ShardedLRU[V]) Add(key string, v V) {
	sl := c.getShard(key)
	sl.Add(key, v)
}

func (c *ShardedLRU[V]) Del(key string) {
	sl := c.getShard(key)
	sl.Del(key)
}

func (c *ShardedLRU[V]) Clean(f func(key string, v V) (remove bool)) (removed int) {
	for i := range c.l {
		removed += c.l[i].Clean(f)
	}
	return removed
}

func (c *ShardedLRU[V]) Get(key string) (v V, ok bool) {
	sl := c.getShard(key)
	v, ok = sl.Get(key)
	return
}

func (c *ShardedLRU[V]) Len() int {
	sum := 0
	for _, shard := range c.l {
		sum += shard.Len()
	}
	return sum
}

func (c *ShardedLRU[V]) shardNum() int {
	return len(c.l)
}

func (c *ShardedLRU[V]) getShard(key string) *ConcurrentLRU[string, V] {
	h := maphash.Hash{}
	h.SetSeed(c.seed)

	h.WriteString(key)
	n := h.Sum64() % uint64(c.shardNum())
	return c.l[n]
}

// ConcurrentLRU is a lru.LRU with a lock.
// It is concurrent safe.
type ConcurrentLRU[K comparable, V any] struct {
	mu   sync.Mutex
	lru  *lru.LRU[K, V]
}

func NewConcurrentLRU[K comparable, V any](maxSize int, onEvict func(key K, v V)) *ConcurrentLRU[K, V] {
	return &ConcurrentLRU[K, V]{
		lru: lru.NewLRU[K, V](maxSize, onEvict),
	}
}

func (c *ConcurrentLRU[K, V]) Add(key K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Add(key, v)
}

func (c *ConcurrentLRU[K, V]) Del(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Del(key)
}

func (c *ConcurrentLRU[K, V]) Clean(f func(key K, v V) (remove bool)) (removed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Clean(f)
}

func (c *ConcurrentLRU[K, V]) Get(key K) (v V, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok = c.lru.Get(key)
	return
}

func (c *ConcurrentLRU[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}