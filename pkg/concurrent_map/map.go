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

package concurrent_map

import (
	"sync"
)

const (
	MapShardSize = 64
)

type Hashable interface {
	comparable
	Sum() uint64
}

type TestAndSetFunc[K comparable, V any] func(key K, v V, ok bool) (newV V, setV, deleteV bool)

type Map[K Hashable, V any] struct {
	shards [MapShardSize]shard[K, V]
}

func NewMap[K Hashable, V any]() *Map[K, V] {
	m := new(Map[K, V])
	for i := range m.shards {
		m.shards[i] = newShard[K, V](0)
	}
	return m
}

// NewMapCache returns a cache with a maximum size.
// Note that, because this it has multiple (MapShardSize) shards,
// the actual maximum size is MapShardSize*(size / MapShardSize).
// If size <=0, it's equal to NewMap().
func NewMapCache[K Hashable, V any](size int) *Map[K, V] {
	sizePreShard := size / MapShardSize
	m := new(Map[K, V])
	for i := range m.shards {
		m.shards[i] = newShard[K, V](sizePreShard)
	}
	return m
}

func (m *Map[K, V]) getShard(key K) *shard[K, V] {
	return &m.shards[key.Sum()%MapShardSize]
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	return m.getShard(key).get(key)
}

func (m *Map[K, V]) Set(key K, v V) {
	m.getShard(key).set(key, v)
}

func (m *Map[K, V]) Del(key K) {
	m.getShard(key).del(key)
}

func (m *Map[K, V]) TestAndSet(key K, f func(v V, ok bool) (newV V, setV, delV bool)) {
	m.getShard(key).testAndSet(key, f)
}

func (m *Map[K, V]) RangeDo(f func(k K, v V) (newV V, setV, delV bool, err error)) error {
	for i := range m.shards {
		if err := m.shards[i].rangeDo(f); err != nil {
			return err
		}
	}
	return nil
}

func (m *Map[K, V]) Len() int {
	l := 0
	for i := range m.shards {
		l += m.shards[i].len()
	}
	return l
}

func (m *Map[K, V]) Flush() {
	for i := range m.shards {
		m.shards[i].flush()
	}
}

type shard[K comparable, V any] struct {
	l   sync.RWMutex
	max int // Negative or zero max means no limit.
	m   map[K]V
}

func newShard[K comparable, V any](max int) shard[K, V] {
	return shard[K, V]{
		max: max,
		m:   make(map[K]V),
	}
}

func (m *shard[K, V]) get(key K) (V, bool) {
	m.l.RLock()
	defer m.l.RUnlock()
	v, ok := m.m[key]
	return v, ok
}

func (m *shard[K, V]) set(key K, v V) {
	m.l.Lock()
	defer m.l.Unlock()
	if m.max > 0 && len(m.m)+1 > m.max {
		for k := range m.m {
			delete(m.m, k)
			if len(m.m)+1 <= m.max {
				break
			}
		}
	}
	m.m[key] = v
}

func (m *shard[K, V]) del(key K) {
	m.l.Lock()
	defer m.l.Unlock()
	delete(m.m, key)
}

func (m *shard[K, V]) testAndSet(key K, f func(v V, ok bool) (newV V, setV, delV bool)) {
	m.l.Lock()
	defer m.l.Unlock()
	v, ok := m.m[key]
	newV, setV, deleteV := f(v, ok)
	switch {
	case setV:
		m.m[key] = newV
	case deleteV && ok:
		delete(m.m, key)
	}
}

func (m *shard[K, V]) len() int {
	m.l.RLock()
	defer m.l.RUnlock()
	return len(m.m)
}

func (m *shard[K, V]) flush() {
	m.l.RLock()
	defer m.l.RUnlock()
	m.m = make(map[K]V)
}

func (m *shard[K, V]) rangeDo(f func(k K, v V) (newV V, setV, delV bool, err error)) error {
	m.l.Lock()
	defer m.l.Unlock()
	for k, v := range m.m {
		newV, setV, deleteV, err := f(k, v)
		if err != nil {
			return err
		}
		switch {
		case setV:
			m.m[k] = newV
		case deleteV:
			delete(m.m, k)
		}
	}
	return nil
}
