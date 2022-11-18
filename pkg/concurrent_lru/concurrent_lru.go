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
	"github.com/IrineSistiana/mosdns/v5/pkg/lru"
	"sync"
)

type Hashable interface {
	comparable
	Sum() uint64
}

type ShardedLRU[K Hashable, V any] struct {
	l []*ConcurrentLRU[K, V]
}

func NewShardedLRU[K Hashable, V any](
	shardNum, maxSizePerShard int,
	onEvict func(key K, v V),
) *ShardedLRU[K, V] {
	cl := &ShardedLRU[K, V]{
		l: make([]*ConcurrentLRU[K, V], 0, shardNum),
	}

	for i := 0; i < shardNum; i++ {
		cl.l = append(cl.l, NewConecurrentLRU[K, V](maxSizePerShard, onEvict))
	}

	return cl
}

func (c *ShardedLRU[K, V]) Add(key K, v V) {
	sl := c.getShard(key)
	sl.Add(key, v)
}

func (c *ShardedLRU[K, V]) Del(key K) {
	sl := c.getShard(key)
	sl.Del(key)
}

func (c *ShardedLRU[K, V]) Clean(f func(key K, v V) (remove bool)) (removed int) {
	for _, l := range c.l {
		removed += l.Clean(f)
	}
	return removed
}

func (c *ShardedLRU[K, V]) Flush() {
	for _, l := range c.l {
		l.Flush()
	}
}

func (c *ShardedLRU[K, V]) Get(key K) (v V, ok bool) {
	sl := c.getShard(key)
	v, ok = sl.Get(key)
	return
}

func (c *ShardedLRU[K, V]) Len() int {
	sum := 0
	for _, l := range c.l {
		sum += l.Len()
	}
	return sum
}

func (c *ShardedLRU[K, V]) shardNum() int {
	return len(c.l)
}

func (c *ShardedLRU[K, V]) getShard(key K) *ConcurrentLRU[K, V] {
	return c.l[key.Sum()%uint64(c.shardNum())]
}

// ConcurrentLRU is a lru.LRU with a lock.
// It is concurrent safe.
type ConcurrentLRU[K comparable, V any] struct {
	sync.Mutex
	lru *lru.LRU[K, V]
}

func NewConecurrentLRU[K comparable, V any](maxSize int, onEvict func(key K, v V)) *ConcurrentLRU[K, V] {
	return &ConcurrentLRU[K, V]{
		lru: lru.NewLRU[K, V](maxSize, onEvict),
	}
}

func (c *ConcurrentLRU[K, V]) Add(key K, v V) {
	c.Lock()
	defer c.Unlock()

	c.lru.Add(key, v)
}

func (c *ConcurrentLRU[K, V]) Del(key K) {
	c.Lock()
	defer c.Unlock()

	c.lru.Del(key)
}

func (c *ConcurrentLRU[K, V]) Clean(f func(key K, v V) (remove bool)) (removed int) {
	c.Lock()
	defer c.Unlock()

	return c.lru.Clean(f)
}

func (c *ConcurrentLRU[K, V]) Flush() {
	c.Lock()
	defer c.Unlock()
	c.lru.Flush()
}

func (c *ConcurrentLRU[K, V]) Get(key K) (v V, ok bool) {
	c.Lock()
	defer c.Unlock()

	v, ok = c.lru.Get(key)
	return
}

func (c *ConcurrentLRU[K, V]) Len() int {
	c.Lock()
	defer c.Unlock()

	return c.lru.Len()
}
