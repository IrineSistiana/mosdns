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
	mapShardSize = 64
)

type MapHashable interface {
	comparable
	MapHash() int
}

type TestAndSetFunc[K comparable, V any] func(key K, v V, ok bool) (newV V, setV, deleteV bool)

type Map[K MapHashable, V any] struct {
	shards [mapShardSize]netipAddrMapShard[K, V]
}

func NewMap[K MapHashable, V any]() *Map[K, V] {
	m := new(Map[K, V])
	for i := range m.shards {
		m.shards[i] = newNetipAddrMapShard[K, V]()
	}
	return m
}

func (m *Map[K, V]) getShard(key K) *netipAddrMapShard[K, V] {
	return &m.shards[key.MapHash()%mapShardSize]
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

func (m *Map[K, V]) TestAndSet(key K, f TestAndSetFunc[K, V]) {
	m.getShard(key).testAndSet(key, f)
}

func (m *Map[K, V]) RangeDo(f TestAndSetFunc[K, V]) {
	for i := range m.shards {
		m.shards[i].rangeDo(f)
	}
}

func (m *Map[K, V]) Len() int {
	l := 0
	for i := range m.shards {
		l += m.shards[i].len()
	}
	return l
}

type netipAddrMapShard[K comparable, V any] struct {
	l sync.RWMutex
	m map[K]V
}

func newNetipAddrMapShard[K comparable, V any]() netipAddrMapShard[K, V] {
	return netipAddrMapShard[K, V]{
		m: make(map[K]V),
	}
}

func (m *netipAddrMapShard[K, V]) get(key K) (V, bool) {
	m.l.RLock()
	defer m.l.RUnlock()
	v, ok := m.m[key]
	return v, ok
}

func (m *netipAddrMapShard[K, V]) set(key K, v V) {
	m.l.Lock()
	defer m.l.Unlock()
	m.m[key] = v
}

func (m *netipAddrMapShard[K, V]) del(key K) {
	m.l.Lock()
	defer m.l.Unlock()
	delete(m.m, key)
}

func (m *netipAddrMapShard[K, V]) testAndSet(key K, f TestAndSetFunc[K, V]) {
	m.l.Lock()
	defer m.l.Unlock()
	v, ok := m.m[key]
	newV, setV, deleteV := f(key, v, ok)
	switch {
	case setV:
		m.m[key] = newV
	case deleteV && ok:
		delete(m.m, key)
	}
}

func (m *netipAddrMapShard[K, V]) len() int {
	m.l.RLock()
	defer m.l.RUnlock()
	return len(m.m)
}

func (m *netipAddrMapShard[K, V]) rangeDo(f TestAndSetFunc[K, V]) {
	m.l.Lock()
	defer m.l.Unlock()
	for k, v := range m.m {
		newV, setV, deleteV := f(k, v, true)
		switch {
		case setV:
			m.m[k] = newV
		case deleteV:
			delete(m.m, k)
		}
	}
}
