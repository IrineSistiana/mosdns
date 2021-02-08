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

//     This file is a modified version from github.com/xtaci/smux/blob/master/alloc.go f386d90
//     license of smux: MIT https://github.com/xtaci/smux/blob/master/LICENSE

package utils

import (
	"fmt"
	"github.com/miekg/dns"
	"math/bits"
	"sync"
)

var (
	defaultAllocator = NewAllocator()
)

type Allocator struct {
	buffers []sync.Pool
}

// NewAllocator initiates a []byte allocator for dns.Msg less than 65536 bytes,
// the waste(memory fragmentation) of space allocation is guaranteed to be
// no more than 50%.
func NewAllocator() *Allocator {
	alloc := new(Allocator)
	alloc.buffers = make([]sync.Pool, 17) // 1B -> 64K
	for k := range alloc.buffers {
		i := k
		alloc.buffers[k].New = func() interface{} {
			return make([]byte, 1<<uint32(i))
		}
	}
	return alloc
}

func GetMsgBuf(size int) []byte {
	return defaultAllocator.Get(size)
}

func PackBuffer(m *dns.Msg) (wire, buf []byte, err error) {
	l := m.Len()
	if l > dns.MaxMsgSize || l <= 0 {
		return nil, nil, fmt.Errorf("msg length %d is invalid", l)
	}
	buf = GetMsgBuf(l)

	wire, err = m.PackBuffer(buf)
	if err != nil {
		ReleaseMsgBuf(buf)
		return nil, nil, err
	}
	return wire, buf, nil
}

func ReleaseMsgBuf(buf []byte) {
	defaultAllocator.Put(buf)
}

// Get a []byte from pool with most appropriate cap
func (alloc *Allocator) Get(size int) []byte {
	if size <= 0 || size > 65536 {
		panic("unexpected size")
	}

	bits := msb(size)
	if size == 1<<bits {
		return alloc.buffers[bits].Get().([]byte)[:size]
	} else {
		return alloc.buffers[bits+1].Get().([]byte)[:size]
	}
}

// Put returns a []byte to pool for future use,
// which the cap must be exactly 2^n
func (alloc *Allocator) Put(buf []byte) {
	bits := msb(cap(buf))
	if cap(buf) == 0 || cap(buf) > 65536 || cap(buf) != 1<<bits {
		panic("unexpected cap size")
	}
	alloc.buffers[bits].Put(buf)
}

// msb return the pos of most significant bit
func msb(size int) int {
	if size == 0 {
		return 0
	}
	return bits.Len32(uint32(size)) - 1
}
