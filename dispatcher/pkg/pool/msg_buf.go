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

package pool

import (
	"fmt"
	"github.com/miekg/dns"
)

// PackBuffer packs the dns msg m to wire format.
// Callers should release the buf after they have done with the wire []byte.
func PackBuffer(m *dns.Msg) (wire []byte, buf *Buffer, err error) {
	l := m.Len()
	if l > dns.MaxMsgSize || l <= 0 {
		return nil, nil, fmt.Errorf("msg length %d is invalid", l)
	}

	// dns.Msg.PackBuffer() needs one more bit than its msg length.
	// It also needs a much larger buffer if the msg is compressed.
	// It is tedious to force dns.Msg.PackBuffer() to use the buffer.
	// Just give it a big buf and hope the buf will be reused in most scenes.
	if l > 4095 {
		buf = GetBuf(l + 1)
	} else {
		buf = GetBuf(4096)
	}

	wire, err = m.PackBuffer(buf.Bytes())
	if err != nil {
		buf.Release()
		return nil, nil, err
	}
	return wire, buf, nil
}
