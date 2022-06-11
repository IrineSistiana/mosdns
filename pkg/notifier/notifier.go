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

package notifier

import (
	"sync"
)

type Notifier struct {
	rwm sync.RWMutex
	c   chan struct{}
}

func NewNotifier() *Notifier {
	return &Notifier{
		c: make(chan struct{}),
	}
}

func (n *Notifier) Wait() <-chan struct{} {
	n.rwm.RLock()
	defer n.rwm.RUnlock()
	return n.c
}

func (n *Notifier) Notify() {
	nc := make(chan struct{})
	n.rwm.Lock()
	oldChan := n.c
	n.c = nc
	n.rwm.Unlock()
	close(oldChan)
}

type DataNotifier[T any] struct {
	rwm sync.RWMutex
	ld  *LazyData[T]
}

type LazyData[T any] struct {
	ready chan struct{}
	d     T
}

func NewLazyData[T any]() *LazyData[T] {
	return &LazyData[T]{
		ready: make(chan struct{}),
	}
}

func (ld *LazyData[T]) Ready() <-chan struct{} {
	return ld.ready
}

func (ld *LazyData[T]) Data() T {
	return ld.d
}

func NewDataNotifier[T any]() *DataNotifier[T] {
	return &DataNotifier[T]{
		ld: &LazyData[T]{
			ready: make(chan struct{}),
		},
	}
}

func (dn *DataNotifier[T]) Wait() *LazyData[T] {
	dn.rwm.RLock()
	defer dn.rwm.RUnlock()
	return dn.ld
}

func (dn *DataNotifier[T]) Notify(d T) {
	newLd := NewLazyData[T]()
	dn.rwm.Lock()
	oldLd := dn.ld
	dn.ld = newLd
	dn.rwm.Unlock()
	oldLd.d = d
	close(oldLd.ready)
}
