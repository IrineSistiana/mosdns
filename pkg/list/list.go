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

package list

type List[V any] struct {
	front, back *Elem[V]
	length      int
}

func New[V any]() *List[V] {
	return &List[V]{}
}

func mustBeFreeElem[V any](e *Elem[V]) {
	if e.prev != nil || e.next != nil || e.list != nil {
		panic("element is in use")
	}
}

func (l *List[V]) Front() *Elem[V] {
	return l.front
}

func (l *List[V]) Back() *Elem[V] {
	return l.back
}

func (l *List[V]) Len() int {
	return l.length
}

func (l *List[V]) PushFront(e *Elem[V]) *Elem[V] {
	mustBeFreeElem(e)
	l.length++
	e.list = l
	if l.front == nil {
		l.front = e
		l.back = e
	} else {
		e.next = l.front
		l.front.prev = e
		l.front = e
	}
	return e
}

func (l *List[V]) PushBack(e *Elem[V]) *Elem[V] {
	mustBeFreeElem(e)
	l.length++
	e.list = l
	if l.back == nil {
		l.front = e
		l.back = e
	} else {
		e.prev = l.back
		l.back.next = e
		l.back = e
	}
	return e
}

func (l *List[V]) PopElem(e *Elem[V]) *Elem[V] {
	if e.list != l {
		panic("elem is not belong to this list")
	}

	l.length--
	if p := e.prev; p != nil {
		p.next = e.next
	}
	if n := e.next; n != nil {
		n.prev = e.prev
	}
	if e == l.front {
		l.front = e.next
	}
	if e == l.back {
		l.back = e.prev
	}
	e.prev, e.next, e.list = nil, nil, nil
	return e
}
