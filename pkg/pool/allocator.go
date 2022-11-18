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

package pool

import (
	"fmt"
	"math"
	"math/bits"
	"sync"
)

// defaultBufPool is an Allocator that has a maximum capacity.
var defaultBufPool = NewAllocator()

// GetBuf returns a []byte from pool with most appropriate cap.
// It panics if size < 0.
func GetBuf(size int) []byte {
	return defaultBufPool.Get(size)
}

// ReleaseBuf puts the buf to the pool.
func ReleaseBuf(b []byte) {
	defaultBufPool.Release(b)
}

type Allocator struct {
	buffers []sync.Pool
}

// NewAllocator initiates a []byte Allocator.
// The waste(memory fragmentation) of space allocation is guaranteed to be
// no more than 50%.
func NewAllocator() *Allocator {
	alloc := &Allocator{
		buffers: make([]sync.Pool, bits.UintSize+1),
	}

	for i := range alloc.buffers {
		var bufSize uint
		if i == bits.UintSize {
			bufSize = math.MaxUint
		} else {
			bufSize = 1 << i
		}
		alloc.buffers[i].New = func() interface{} {
			b := make([]byte, bufSize)
			return &b
		}
	}
	return alloc
}

// Get returns a []byte from pool with most appropriate cap
func (alloc *Allocator) Get(size int) []byte {
	if size < 0 {
		panic(fmt.Sprintf("invalid slice size %d", size))
	}

	i := shard(size)
	v := alloc.buffers[i].Get()
	buf := v.(*[]byte)
	return (*buf)[0:size]
}

// Release releases the buf to the allocatorL.
func (alloc *Allocator) Release(buf []byte) {
	c := cap(buf)
	i := shard(c)
	if c == 0 || c != 1<<i {
		panic("unexpected cap size")
	}
	alloc.buffers[i].Put(&buf)
}

// shard returns the shard index that is suitable for the size.
func shard(size int) int {
	if size <= 1 {
		return 0
	}
	return bits.Len64(uint64(size - 1))
}
