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
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/pool"
	"github.com/miekg/dns"
	"io"
	"net"
)

const (
	IPv4UdpMaxPayload = 1472 // MTU 1500 - 20 IPv4 header - 8 udp header
	IPv6UdpMaxPayload = 1452 // MTU 1500 - 40 IPv6 header - 8 udp header
)

type IOErr struct {
	err error
}

func WrapIOErr(err error) *IOErr {
	return &IOErr{err: err}
}

func (e *IOErr) Error() string {
	return e.err.Error()
}

func IsIOErr(err error) (innerErr error) {
	innerErr, ok := err.(*IOErr)
	if ok {
		return innerErr
	}
	return nil
}

func (e *IOErr) Unwrap() error {
	return e.err
}

// ReadUDPMsgFrom reads dns msg from c in a wire format.
// The bufSize cannot be greater than dns.MaxMsgSize.
// Typically IPv4UdpMaxPayload is big enough.
// An io err will be wrapped into an IOErr.
// IsIOErr(err) can check and unwrap the inner io err.
func ReadUDPMsgFrom(c net.PacketConn, bufSize int) (m *dns.Msg, from net.Addr, n int, err error) {
	buf := pool.GetMsgBuf(bufSize)
	defer pool.ReleaseMsgBuf(buf)

	n, from, err = c.ReadFrom(buf)
	if err != nil {
		err = WrapIOErr(err)
		return
	}

	if n < 12 {
		err = dns.ErrShortRead
		return
	}

	m = new(dns.Msg)
	err = m.Unpack(buf[:n])
	if err != nil {
		return
	}
	return
}

// ReadMsgFromUDP See ReadUDPMsgFrom.
func ReadMsgFromUDP(c io.Reader, bufSize int) (m *dns.Msg, n int, err error) {
	buf := pool.GetMsgBuf(bufSize)
	defer pool.ReleaseMsgBuf(buf)

	n, err = c.Read(buf)
	if err != nil {
		err = WrapIOErr(err)
		return nil, n, err
	}
	if n < 12 {
		return nil, n, dns.ErrShortRead
	}

	m = new(dns.Msg)
	err = m.Unpack(buf[:n])
	if err != nil {
		return nil, n, err
	}
	return m, n, nil
}

// WriteMsgToUDP packs and writes m to c in a wire format.
// An io err will be wrapped into an IOErr.
// IsIOErr(err) can check and unwrap the inner io err.
func WriteMsgToUDP(c io.Writer, m *dns.Msg) (n int, err error) {
	mRaw, buf, err := pool.PackBuffer(m)
	if err != nil {
		return 0, err
	}
	defer pool.ReleaseMsgBuf(buf)

	return WriteRawMsgToUDP(c, mRaw)
}

// WriteRawMsgToUDP See WriteMsgToUDP.
func WriteRawMsgToUDP(c io.Writer, b []byte) (n int, err error) {
	n, err = c.Write(b)
	if err != nil {
		err = WrapIOErr(err)
	}
	return
}

// WriteUDPMsgTo See WriteMsgToUDP.
func WriteUDPMsgTo(m *dns.Msg, c net.PacketConn, to net.Addr) (n int, err error) {
	mRaw, buf, err := pool.PackBuffer(m)
	if err != nil {
		return 0, err
	}
	defer pool.ReleaseMsgBuf(buf)

	n, err = c.WriteTo(mRaw, to)
	if err != nil {
		err = WrapIOErr(err)
	}
	return
}

// ReadMsgFromTCP reads msg from c in RFC 7766 format.
// n represents how many bytes are read from c.
// This includes two-octet length field.
// An io err will be wrapped into an IOErr.
// IsIOErr(err) can check and unwrap the inner io err.
func ReadMsgFromTCP(c io.Reader) (m *dns.Msg, n int, err error) {
	lengthRaw := make([]byte, 2)
	n1, err := io.ReadFull(c, lengthRaw)
	n = n + n1
	if err != nil {
		err = WrapIOErr(err)
		return nil, n, err
	}

	// dns length
	length := binary.BigEndian.Uint16(lengthRaw)
	if length < 12 {
		return nil, n, dns.ErrShortRead
	}

	buf := pool.GetMsgBuf(int(length))
	defer pool.ReleaseMsgBuf(buf)

	n2, err := io.ReadFull(c, buf)
	n = n + n2
	if err != nil {
		err = WrapIOErr(err)
		return nil, n, err
	}

	m = new(dns.Msg)
	err = m.Unpack(buf)
	if err != nil {
		return nil, n, err
	}
	return m, n, nil
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
	defer pool.ReleaseMsgBuf(buf)

	return WriteRawMsgToTCP(c, mRaw)
}

// WriteRawMsgToTCP See WriteMsgToTCP
func WriteRawMsgToTCP(c io.Writer, b []byte) (n int, err error) {
	if len(b) > dns.MaxMsgSize {
		return 0, fmt.Errorf("payload length %d is greater than dns max msg size", len(b))
	}

	if tcpConn, ok := c.(*net.TCPConn); ok {
		h := make([]byte, 2)
		h[0] = byte(len(b) >> 8)
		h[1] = byte(len(b))
		buf := net.Buffers{h, b}
		wn, err := buf.WriteTo(tcpConn)
		return int(wn), err
	}

	wb := tcpWriteBufPool.Get()
	defer tcpWriteBufPool.Release(wb)
	wb.WriteByte(byte(len(b) >> 8))
	wb.WriteByte(byte(len(b)))
	wb.Write(b)
	n, err = c.Write(wb.Bytes())
	if err != nil {
		err = WrapIOErr(err)
	}
	return
}

var (
	tcpWriteBufPool = pool.NewBytesBufPool(512 + 2)
)
