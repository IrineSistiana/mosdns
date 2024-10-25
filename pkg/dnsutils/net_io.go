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

package dnsutils

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/pool"
	"github.com/miekg/dns"
	"io"
)

var (
	errZeroLenMsg = errors.New("zero length msg")
)

// ReadRawMsgFromTCP reads msg from c in RFC 1035 format (msg is prefixed
// with a two byte length field).
// n represents how many bytes are read from c.
func ReadRawMsgFromTCP(c io.Reader) (*pool.Buffer, int, error) {
	var n int
	header := make([]byte, 2)
	if _, err := io.ReadFull(c, header); err != nil {
		return nil, n, err
	}
	n += 2

	length := binary.BigEndian.Uint16(header)
	if length == 0 {
		return nil, 0, errZeroLenMsg
	}

	buf := pool.GetBuf(int(length))
	defer func() {
		if buf.Len() == 0 {
			buf.Release()
		}
	}()

	if _, err := io.ReadFull(c, buf.Bytes()); err != nil {
		return nil, n, err
	}
	n += int(length)
	buf.SetLen(int(length))
	return buf, n, nil
}

// ReadMsgFromTCP reads msg from c in RFC 1035 format (msg is prefixed
// with a two byte length field).
// n represents how many bytes are read from c.
func ReadMsgFromTCP(c io.Reader) (*dns.Msg, int, error) {
	buf, n, err := ReadRawMsgFromTCP(c)
	if err != nil {
		return nil, 0, err
	}
	defer buf.Release()

	m, err := unpackMsgWithDetailedErr(buf.Bytes())
	return m, n, err
}

// WriteMsgToTCP packs and writes m to c in RFC 1035 format.
// n represents how many bytes are written to c.
func WriteMsgToTCP(c io.Writer, m *dns.Msg) (n int, err error) {
	mRaw, buf, err := pool.PackBuffer(m)
	if err != nil {
		return 0, err
	}
	defer buf.Release()
	return WriteRawMsgToTCP(c, mRaw)
}

// WriteRawMsgToTCP writes raw DNS message to c in RFC 1035 format.
// n represents how many bytes are written to c.
func WriteRawMsgToTCP(c io.Writer, b []byte) (n int, err error) {
	if len(b) > dns.MaxMsgSize {
		return 0, fmt.Errorf("payload length %d is greater than dns max msg size", len(b))
	}

	buf := pool.GetBuf(len(b) + 2)
	defer buf.Release()
	wb := buf.Bytes()

	binary.BigEndian.PutUint16(wb[:2], uint16(len(b)))
	copy(wb[2:], b)
	return c.Write(wb)
}

// WriteMsgToUDP packs and writes m to c.
// n represents how many bytes are written to c.
func WriteMsgToUDP(c io.Writer, m *dns.Msg) (int, error) {
	b, buf, err := pool.PackBuffer(m)
	if err != nil {
		return 0, err
	}
	defer buf.Release()

	return c.Write(b)
}

// ReadMsgFromUDP reads msg from c.
// n represents how many bytes are read from c.
func ReadMsgFromUDP(c io.Reader, bufSize int) (*dns.Msg, int, error) {
	if bufSize < dns.MinMsgSize {
		bufSize = dns.MinMsgSize
	}

	buf := pool.GetBuf(bufSize)
	defer func() {
		if buf.Len() == 0 {
			buf.Release()
		}
	}()

	n, err := c.Read(buf.Bytes())
	if err != nil {
		return nil, n, err
	}

	m, err := unpackMsgWithDetailedErr(buf.Bytes()[:n])
	return m, n, err
}

func unpackMsgWithDetailedErr(b []byte) (*dns.Msg, error) {
	m := new(dns.Msg)
	if err := m.Unpack(b); err != nil {
		return nil, fmt.Errorf("failed to unpack msg [%x], %w", b, err)
	}
	return m, nil
}