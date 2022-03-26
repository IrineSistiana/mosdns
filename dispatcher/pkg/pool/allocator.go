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
	"math"
	"math/bits"
	"sync"
)

const intSize = 32 << (^uint(0) >> 63)

// defaultBufPool is an Allocator that has a maximum capacity.
var defaultBufPool = NewAllocator(intSize - 1)

// GetBuf returns a *Buffer from pool with most appropriate cap.
// It panics if size < 0.
func GetBuf(size int) *Buffer {
	return defaultBufPool.Get(size)
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
	if maxPoolBitsLen > intSize-1 || maxPoolBitsLen <= 0 {
		panic("invalid pool length")
	}

	ml := 1 << maxPoolBitsLen
	if maxPoolBitsLen == intSize-1 {
		ml = math.MaxInt
	}
	alloc := &Allocator{
		maxPoolLen: ml,
		buffers:    make([]sync.Pool, maxPoolBitsLen+1),
	}

	for i := range alloc.buffers {
		var bufSize int
		if i == intSize-1 {
			bufSize = math.MaxInt
		} else {
			bufSize = 1 << i
		}
		alloc.buffers[i].New = func() interface{} {
			return newBuffer(alloc, make([]byte, bufSize))
		}
	}
	return alloc
}

// Get returns a []byte from pool with most appropriate cap
func (alloc *Allocator) Get(size int) *Buffer {
	if size < 0 {
		panic(fmt.Sprintf("invalid slice size %d", size))
	}

	if size > alloc.maxPoolLen {
		panic(fmt.Sprintf("slice size %d is too large", size))
	}

	i := shard(size)
	buf := alloc.buffers[i].Get().(*Buffer)
	buf.SetLen(size)
	return buf
}

// Release releases the buf to the allocatorL.
func (alloc *Allocator) Release(buf *Buffer) {
	c := buf.Cap()
	i := shard(c)
	if c == 0 || c > alloc.maxPoolLen || c != 1<<i {
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

type Buffer struct {
	a *Allocator

	l int
	b []byte
}

func newBuffer(a *Allocator, b []byte) *Buffer {
	return &Buffer{
		a: a,
		l: len(b),
		b: b,
	}
}

func (b *Buffer) SetLen(l int) {
	if l > len(b.b) {
		panic("buffer length overflowed")
	}
	b.l = l
}

func (b *Buffer) AllBytes() []byte {
	return b.b
}

func (b *Buffer) Bytes() []byte {
	return b.b[:b.l]
}

func (b *Buffer) Len() int {
	return b.l
}

func (b *Buffer) Cap() int {
	return cap(b.b)
}

func (b *Buffer) Release() {
	b.a.Release(b)
}
