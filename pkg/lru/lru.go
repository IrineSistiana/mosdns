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

package lru

import (
	"fmt"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/list"
)

type LRU[K comparable, V any] struct {
	maxSize int
	onEvict func(key K, v V)

	l *list.List[KV[K, V]]
	m map[K]*list.Elem[KV[K, V]]
}

type KV[K comparable, V any] struct {
	key K
	v   V
}

func NewLRU[K comparable, V any](maxSize int, onEvict func(key K, v V)) *LRU[K, V] {
	if maxSize <= 0 {
		panic(fmt.Sprintf("LRU: invalid max size: %d", maxSize))
	}

	return &LRU[K, V]{
		maxSize: maxSize,
		onEvict: onEvict,
		l:       list.New[KV[K, V]](),
		m:       make(map[K]*list.Elem[KV[K, V]]),
	}
}

func (q *LRU[K, V]) Add(key K, v V) {
	if e, ok := q.m[key]; ok { // update existed key
		e.Value.v = v
		q.l.PushBack(q.l.PopElem(e))
		return
	}

	o := q.Len() - q.maxSize + 1
	for o > 0 {
		key, v, _ := q.PopOldest()
		if q.onEvict != nil {
			q.onEvict(key, v)
		}
		o--
	}

	e := list.NewElem(KV[K, V]{
		key: key,
		v:   v,
	})
	q.m[key] = e
	q.l.PushBack(e)
}

func (q *LRU[K, V]) Del(key K) {
	e := q.m[key]
	if e != nil {
		q.delElem(e)
	}
}

func (q *LRU[K, V]) delElem(e *list.Elem[KV[K, V]]) {
	key, v := e.Value.key, e.Value.v
	q.l.PopElem(e)
	delete(q.m, key)
	if q.onEvict != nil {
		q.onEvict(key, v)
	}
}

func (q *LRU[K, V]) PopOldest() (key K, v V, ok bool) {
	e := q.l.Front()
	if e != nil {
		q.l.PopElem(e)
		key, v = e.Value.key, e.Value.v
		delete(q.m, key)
		ok = true
		return
	}
	return
}

func (q *LRU[K, V]) Clean(f func(key K, v V) (remove bool)) (removed int) {
	e := q.l.Front()
	for e != nil {
		next := e.Next() // Delete e will clean its pointers. Save it first.
		key, v := e.Value.key, e.Value.v
		if remove := f(key, v); remove {
			q.delElem(e)
			removed++
		}
		e = next
	}
	return removed
}

func (q *LRU[K, V]) Get(key K) (v V, ok bool) {
	e, ok := q.m[key]
	if !ok {
		return
	}
	q.l.PushBack(q.l.PopElem(e))
	return e.Value.v, true
}

func (q *LRU[K, V]) Len() int {
	return q.l.Len()
}
