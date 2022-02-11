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

package dnsutils

import (
	"encoding/binary"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/pool"
	"github.com/miekg/dns"
	"io"
)

// ReadRawMsgFromTCP reads msg from c in RFC 7766 format.
// n represents how many bytes are read from c.
// This includes two-octet length field.
func ReadRawMsgFromTCP(c io.Reader) (*pool.Buffer, int, error) {
	n := 0
	hb := pool.GetBuf(2)
	defer hb.Release()
	h := hb.Bytes()
	nh, err := io.ReadFull(c, h)
	n += nh
	if err != nil {
		return nil, n, err
	}

	// dns length
	length := binary.BigEndian.Uint16(h)
	buf := pool.GetBuf(int(length))

	nm, err := io.ReadFull(c, buf.Bytes())
	n += nm
	if err != nil {
		buf.Release()
		return nil, n, err
	}
	buf.SetLen(nm)
	return buf, n, nil
}

// WriteMsgToTCP packs and writes m to c in RFC 7766 format.
// n represents how many bytes are written to c.
// This includes 2 bytes length header.
// An io err will be wrapped into an IOErr.
// IsIOErr(err) can check and unwrap the inner io err.
func WriteMsgToTCP(c io.Writer, m *dns.Msg) (n int, err error) {
	mRaw, buf, err := pool.PackBuffer(m)
	if err != nil {
		return 0, err
	}
	defer buf.Release()

	return WriteRawMsgToTCP(c, mRaw)
}

// WriteRawMsgToTCP See WriteMsgToTCP
func WriteRawMsgToTCP(c io.Writer, b []byte) (n int, err error) {
	if len(b) > dns.MaxMsgSize {
		return 0, fmt.Errorf("payload length %d is greater than dns max msg size", len(b))
	}

	bb := pool.GetBuf(len(b) + 2)
	defer bb.Release()
	wb := bb.Bytes()

	binary.BigEndian.PutUint16(wb[:2], uint16(len(b)))
	copy(wb[2:], b)
	return c.Write(wb)
}
