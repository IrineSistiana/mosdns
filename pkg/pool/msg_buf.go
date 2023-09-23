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
	"encoding/binary"
	"fmt"

	"github.com/miekg/dns"
)

// There is no such way to give dns.Msg.PackBuffer() a buffer
// with a proper size.
// Just give it a big buf and hope the buf will be reused in most scenes.
const packBufferSize = 4097

// PackBuffer packs the dns msg m to wire format.
// Callers should release the buf by calling ReleaseBuf after they have done
// with the wire []byte.
func PackBuffer(m *dns.Msg) (*[]byte, error) {
	b := GetBuf(packBufferSize)
	wire, err := m.PackBuffer(*b)
	if err != nil {
		ReleaseBuf(b)
		return nil, err
	}
	if &((*b)[0]) != &wire[0] { // reallocated
		ReleaseBuf(b)
		b = GetBuf(len(wire))
		copy(*b, wire)
	} else {
		*b = (*b)[:len(wire)]
	}
	return b, nil
}

// PackBuffer packs the dns msg m to wire format, with to bytes length header.
// Callers should release the buf by calling ReleaseBuf.
func PackTCPBuffer(m *dns.Msg) (*[]byte, error) {
	b := GetBuf(packBufferSize)
	wire, err := m.PackBuffer((*b)[2:])
	if err != nil {
		ReleaseBuf(b)
		return nil, err
	}

	l := len(wire)
	if l > dns.MaxMsgSize {
		ReleaseBuf(b)
		return nil, fmt.Errorf("dns payload size %d is too large", l)
	}

	if &((*b)[2]) != &wire[0] { // reallocated
		ReleaseBuf(b)
		b = GetBuf(2 + len(wire))
		copy((*b)[2:], wire)
	} else {
		*b = (*b)[:2+l]
	}
	binary.BigEndian.PutUint16((*b)[:2], uint16(l))
	return b, nil
}
