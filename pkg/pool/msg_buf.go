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
const packBufferSize = 8191

// PackBuffer packs the dns msg m to wire format.
// Callers should release the buf by calling ReleaseBuf after they have done
// with the wire []byte.
func PackBuffer(m *dns.Msg) (*[]byte, error) {
	packBuf := GetBuf(packBufferSize)
	defer ReleaseBuf(packBuf)
	wire, err := m.PackBuffer(*packBuf)
	if err != nil {
		return nil, err
	}

	msgBuf := GetBuf(len(wire))
	copy(*msgBuf, wire)
	return msgBuf, nil
}

// PackBuffer packs the dns msg m to wire format, with to bytes length header.
// Callers should release the buf by calling ReleaseBuf.
func PackTCPBuffer(m *dns.Msg) (*[]byte, error) {
	packBuf := GetBuf(packBufferSize)
	defer ReleaseBuf(packBuf)
	wire, err := m.PackBuffer((*packBuf)[2:])
	if err != nil {
		return nil, err
	}

	l := len(wire)
	if l > dns.MaxMsgSize {
		return nil, fmt.Errorf("dns payload size %d is too large", l)
	}

	msgBuf := GetBuf(2 + len(wire))
	binary.BigEndian.PutUint16(*msgBuf, uint16(l))
	copy((*msgBuf)[2:], wire)
	return msgBuf, nil
}
