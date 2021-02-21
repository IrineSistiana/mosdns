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
	"math/bits"
	"sync"
)

var (
	// allocator is an allocator with maximum buf size limit 4GB (1<<32).
	allocator = NewAllocator(32)
)

// GetBuf returns a buf from allocator1m.
// The size limit is 4GB.
func GetBuf(size int) []byte {
	return allocator.Get(size)
}

// ReleaseBuf releases the b
func ReleaseBuf(b []byte) {
	allocator.Release(b)
}

type Allocator struct {
	maxLen  int
	buffers []sync.Pool
}

// NewAllocator initiates a []byte allocator less than 1 << maxBitsLen bytes,
// the waste(memory fragmentation) of space allocation is guaranteed to be
// no more than 50%.
func NewAllocator(maxBitsLen int) *Allocator {
	alloc := &Allocator{
		maxLen:  1 << maxBitsLen,
		buffers: make([]sync.Pool, maxBitsLen+1),
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
	if size <= 0 || size > alloc.maxLen {
		panic("unexpected size")
	}

	i := shard(size)
	return alloc.buffers[i].Get().([]byte)[:size]
}

// Release releases the buf to Allocator.
func (alloc *Allocator) Release(buf []byte) {
	i := shard(cap(buf))
	if cap(buf) == 0 || cap(buf) > alloc.maxLen || cap(buf) != 1<<i {
		panic("unexpected cap size")
	}
	alloc.buffers[i].Put(buf)
}

// MaxLen returns the Allocator maximum buf length.
func (alloc *Allocator) MaxLen() int {
	return alloc.maxLen
}

// shard returns the shard index that is suitable for the size.
func shard(size int) int {
	if size <= 1 {
		return 0
	}
	return bits.Len64(uint64(size - 1))
}
