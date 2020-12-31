//     Copyright (C) 2020, IrineSistiana
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

package utils

import (
	"bytes"
	"github.com/miekg/dns"
	"sync"
)

var (
	tcpHeaderBufPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 2)
		},
	}

	tcpWriteBufPool = sync.Pool{
		New: func() interface{} {
			b := new(bytes.Buffer)
			b.Grow(dns.MinMsgSize)
			return b
		},
	}
)

func getTCPHeaderBuf() []byte {
	return tcpHeaderBufPool.Get().([]byte)
}

func releaseTCPHeaderBuf(buf []byte) {
	tcpHeaderBufPool.Put(buf)
}

// getTCPWriteBuf returns a byte.Buffer
func getTCPWriteBuf() *bytes.Buffer {
	return tcpWriteBufPool.Get().(*bytes.Buffer)
}

func releaseTCPWriteBuf(buf *bytes.Buffer) {
	buf.Reset()
	tcpWriteBufPool.Put(buf)
}

func packMsgWithBuffer(m *dns.Msg) (mRaw, buf []byte, err error) {
	buf, err = GetMsgBufFor(m)
	if err != nil {
		return
	}

	mRaw, err = m.PackBuffer(buf)
	if err != nil {
		ReleaseMsgBuf(buf)
		return
	}
	return
}
