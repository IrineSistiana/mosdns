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
	"bytes"
	"fmt"
	"sync"
)

type BytesBufPool struct {
	p sync.Pool
}

func NewBytesBufPool(initSize int) *BytesBufPool {
	if initSize < 0 {
		panic(fmt.Sprintf("utils.NewBytesBufPool: negative init size %d", initSize))
	}

	return &BytesBufPool{
		p: sync.Pool{New: func() any {
			b := new(bytes.Buffer)
			b.Grow(initSize)
			return b
		}},
	}
}

func (p *BytesBufPool) Get() *bytes.Buffer {
	return p.p.Get().(*bytes.Buffer)
}

func (p *BytesBufPool) Release(b *bytes.Buffer) {
	b.Reset()
	p.p.Put(b)
}
