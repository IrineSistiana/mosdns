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

package concurrent_limiter

import (
	"github.com/IrineSistiana/mosdns/v4/pkg/concurrent_map"
)

type ClientQueryLimiter struct {
	maxQueries int
	m          *concurrent_map.ConcurrentMap
}

func NewClientQueryLimiter(maxQueries int) *ClientQueryLimiter {
	return &ClientQueryLimiter{
		maxQueries: maxQueries,
		m:          concurrent_map.NewConcurrentMap(64),
	}
}

func (l *ClientQueryLimiter) Acquire(key string) bool {
	return l.m.TestAndSet(key, l.acquireTestAndSet)
}

func (l *ClientQueryLimiter) acquireTestAndSet(v interface{}, ok bool) (newV interface{}, wantUpdate, passed bool) {
	n := 0
	if ok {
		n = v.(int)
	}
	if n >= l.maxQueries {
		return nil, false, false
	}
	n++
	return n, true, true
}

func (l *ClientQueryLimiter) doneTestAndSet(v interface{}, ok bool) (newV interface{}, wantUpdate, passed bool) {
	if !ok {
		panic("ClientQueryLimiter doneTestAndSet: value is not exist")
	}
	n := v.(int)
	n--
	if n < 0 {
		panic("ClientQueryLimiter doneTestAndSet: value becomes negative")
	}
	if n == 0 {
		return nil, true, true
	}
	return n, true, true
}

func (l *ClientQueryLimiter) Done(key string) {
	l.m.TestAndSet(key, l.doneTestAndSet)
}
