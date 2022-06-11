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
	"github.com/miekg/dns"
)

// There is no such way to give dns.Msg.PackBuffer() a buffer
// with a proper size.
// Just give it a big buf and hope the buf will be reused in most scenes.
const packBufSize = 4096

// PackBuffer packs the dns msg m to wire format.
// Callers should release the buf after they have done with the wire []byte.
func PackBuffer(m *dns.Msg) (wire []byte, buf *Buffer, err error) {
	buf = GetBuf(packBufSize)
	wire, err = m.PackBuffer(buf.Bytes())
	if err != nil {
		buf.Release()
		return nil, nil, err
	}
	return wire, buf, nil
}
