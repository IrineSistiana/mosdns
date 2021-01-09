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

package utils

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/miekg/dns"
)

// ReadMsgFromTCP reads msg from a tcp connection.
// n represents how many bytes are read from c.
func ReadMsgFromTCP(c io.Reader) (m *dns.Msg, n int, err error) {
	lengthRaw := getTCPHeaderBuf()
	defer releaseTCPHeaderBuf(lengthRaw)

	n1, err := io.ReadFull(c, lengthRaw)
	n = n + n1
	if err != nil {
		return nil, n, err
	}

	// dns length
	length := binary.BigEndian.Uint16(lengthRaw)
	if length < 12 {
		return nil, n, dns.ErrShortRead
	}

	buf := GetMsgBuf(int(length))
	defer ReleaseMsgBuf(buf)

	n2, err := io.ReadFull(c, buf)
	n = n + n2
	if err != nil {
		return nil, n, err
	}

	m = new(dns.Msg)
	err = m.Unpack(buf)
	if err != nil {
		return nil, n, err
	}
	return m, n, nil
}

// WriteMsgToTCP writes m to c.
// n represents how many bytes are wrote to c. This includes 2 bytes tcp header.
func WriteMsgToTCP(c io.Writer, m *dns.Msg) (n int, err error) {
	mRaw, buf, err := packMsgWithBuffer(m)
	if err != nil {
		return 0, err
	}
	defer ReleaseMsgBuf(buf)

	return WriteRawMsgToTCP(c, mRaw)
}

// WriteRawMsgToTCP writes b to c.
// n represents how many bytes are wrote to c. This includes 2 bytes tcp header.
func WriteRawMsgToTCP(c io.Writer, b []byte) (n int, err error) {
	if len(b) > dns.MaxMsgSize {
		return 0, fmt.Errorf("payload length %d is greater than dns max msg size", len(b))
	}

	wb := getTCPWriteBuf()
	defer releaseTCPWriteBuf(wb)
	wb.WriteByte(byte(len(b) >> 8))
	wb.WriteByte(byte(len(b)))
	wb.Write(b)
	return c.Write(wb.Bytes())
}
