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

package lru

import (
	"container/list"
	"fmt"
)

type LRULike interface {
	Add(key string, v interface{})
	Del(key string)
	Clean(f func(key string, v interface{}) (remove bool)) (removed int)
	Get(key string) (v interface{}, ok bool)
	Len() int
}

type LRU struct {
	maxSize int
	onEvict func(key string, v interface{})

	l *list.List
	m map[string]*list.Element
}

type listValue struct {
	key string
	v   interface{}
}

func NewLRU(maxSize int, onEvict func(key string, v interface{})) *LRU {
	if maxSize <= 0 {
		panic(fmt.Sprintf("LRU: invalid max size: %d", maxSize))
	}

	return &LRU{
		maxSize: maxSize,
		onEvict: onEvict,
		l:       list.New(),
		m:       make(map[string]*list.Element),
	}
}

func (q *LRU) Add(key string, v interface{}) {
	e, ok := q.m[key]
	if ok { // update existed key
		e.Value.(*listValue).v = v
		q.l.MoveToBack(e)
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

	q.m[key] = q.l.PushBack(&listValue{
		key: key,
		v:   v,
	})
}

func (q *LRU) Del(key string) {
	e := q.m[key]
	if e != nil {
		q.mustDel(key, e)
	}
}

func (q *LRU) mustDel(key string, e *list.Element) {
	lv := q.l.Remove(e).(*listValue)
	delete(q.m, key)
	if q.onEvict != nil {
		q.onEvict(key, lv.v)
	}
}

func (q *LRU) PopOldest() (key string, v interface{}, ok bool) {
	e := q.l.Front()
	if e != nil {
		lv := q.l.Remove(e).(*listValue)
		delete(q.m, lv.key)
		key, v = lv.key, lv.v
		ok = true
		return
	}
	return "", nil, false
}

func (q *LRU) Clean(f func(key string, v interface{}) (remove bool)) (removed int) {
	next := q.l.Front()
	for next != nil {
		e := next
		next = e.Next()
		lv := e.Value.(*listValue)
		key, v := lv.key, lv.v
		if remove := f(key, v); remove {
			q.mustDel(key, e)
			removed++
		}
	}
	return removed
}

func (q *LRU) Get(key string) (interface{}, bool) {
	e, ok := q.m[key]
	if !ok {
		return nil, false
	}
	q.l.MoveToBack(e)
	return e.Value.(*listValue).v, true
}

func (q *LRU) Len() int {
	return q.l.Len()
}
