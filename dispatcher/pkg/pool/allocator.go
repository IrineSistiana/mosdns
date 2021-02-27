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

//     This file is a modified version from https://github.com/xtaci/smux/blob/master/alloc.go f386d90
//     license of smux: MIT https://github.com/xtaci/smux/blob/master/LICENSE

package pool

import (
	"fmt"
	"math/bits"
	"sync"
)

const (
	// Since go 1.15, the go memory allocator is much more faster than a sync.Pool
	// when allocating small object.
	ignoreSmallObj = 64
)

var (
	// allocator is an Allocator with 1 Megabyte maximum reusable buf size limit (1<<20).
	allocator = NewAllocator(20)
)

// GetBuf returns a buf from a global allocator.
// The reuse limit is 1 Megabytes.
func GetBuf(size int) []byte {
	return allocator.Get(size)
}

// ReleaseBuf releases the b
func ReleaseBuf(b []byte) {
	allocator.Release(b)
}

type Allocator struct {
	maxPoolLen int
	buffers    []sync.Pool
}

// NewAllocator initiates a []byte allocatorL.
// []byte that has less than 1 << maxPoolBitsLen bytes is managed by sync.Pool.
// The waste(memory fragmentation) of space allocation is guaranteed to be
// no more than 50%.
func NewAllocator(maxPoolBitsLen int) *Allocator {
	alloc := &Allocator{
		maxPoolLen: 1 << maxPoolBitsLen,
		buffers:    make([]sync.Pool, maxPoolBitsLen+1),
	}
	for i := range alloc.buffers {
		bufSize := 1 << uint32(i)
		alloc.buffers[i].New = func() interface{} {
			return make([]byte, bufSize)
		}
	}
	return alloc
}

// Get returns a []byte from pool with most appropriate cap
func (alloc *Allocator) Get(size int) []byte {
	if size <= 0 {
		panic(fmt.Sprintf("Allocator Get: negtive slice size %d", size))
	}

	if size > alloc.maxPoolLen || size <= ignoreSmallObj {
		return make([]byte, size)
	}

	i := shard(size)
	return alloc.buffers[i].Get().([]byte)[:size]
}

// Release releases the buf to the allocatorL.
func (alloc *Allocator) Release(buf []byte) {
	if cap(buf) > alloc.maxPoolLen || cap(buf) <= ignoreSmallObj {
		return
	}

	i := shard(cap(buf))
	if cap(buf) == 0 || cap(buf) > alloc.maxPoolLen || cap(buf) != 1<<i {
		panic("unexpected cap size")
	}
	alloc.buffers[i].Put(buf)
}

// shard returns the shard index that is suitable for the size.
func shard(size int) int {
	if size <= 1 {
		return 0
	}
	return bits.Len64(uint64(size - 1))
}
