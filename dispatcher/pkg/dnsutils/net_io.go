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

// ReadRawUDPMsgFrom reads dns msg from c in a wire format.
// The bufSize should not be greater than dns.MaxMsgSize.
// Typically, IPv4UdpMaxPayload is big enough.
func ReadRawUDPMsgFrom(c net.PacketConn, bufSize int) ([]byte, net.Addr, int, error) {
	buf := pool.GetBuf(bufSize)
	defer pool.ReleaseBuf(buf)

	n, from, err := c.ReadFrom(buf)
	if err != nil {
		return nil, from, n, err
	}

	m := make([]byte, n)
	copy(m, buf)
	return m, from, n, nil
}

// ReadUDPMsgFrom reads dns msg from c in a wire format.
// The bufSize should not be greater than dns.MaxMsgSize.
// Typically, IPv4UdpMaxPayload is big enough.
// An io err will be wrapped into an IOErr.
// IsIOErr(err) can check and unwrap the inner io err.
func ReadUDPMsgFrom(c net.PacketConn, bufSize int) (m *dns.Msg, from net.Addr, n int, err error) {
	buf := pool.GetBuf(bufSize)
	defer pool.ReleaseBuf(buf)

	n, from, err = c.ReadFrom(buf)
	if err != nil {
		err = WrapIOErr(err)
		return
	}

	m = new(dns.Msg)
	err = m.Unpack(buf[:n])
	if err != nil {
		return
	}
	return
}

// ReadRawMsgFromUDP reads dns msg from c in a wire format.
// The bufSize should not be greater than dns.MaxMsgSize.
// Typically, IPv4UdpMaxPayload is big enough.
func ReadRawMsgFromUDP(c io.Reader, bufSize int) ([]byte, int, error) {
	buf := pool.GetBuf(bufSize)
	defer pool.ReleaseBuf(buf)

	n, err := c.Read(buf)
	if err != nil {
		return nil, n, err
	}
	m := make([]byte, n)
	copy(m, buf)
	return m, n, nil
}

// ReadMsgFromUDP See ReadUDPMsgFrom.
func ReadMsgFromUDP(c io.Reader, bufSize int) (*dns.Msg, int, error) {
	buf := pool.GetBuf(bufSize)
	defer pool.ReleaseBuf(buf)

	n, err := c.Read(buf)
	if err != nil {
		err = WrapIOErr(err)
		return nil, n, err
	}

	m := new(dns.Msg)
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
	defer pool.ReleaseBuf(buf)

	return WriteRawMsgToUDP(c, mRaw)
}

// WriteRawMsgToUDP writes b to c.
func WriteRawMsgToUDP(c io.Writer, b []byte) (n int, err error) {
	return c.Write(b)
}

// WriteUDPMsgTo See WriteMsgToUDP.
func WriteUDPMsgTo(m *dns.Msg, c net.PacketConn, to net.Addr) (n int, err error) {
	mRaw, buf, err := pool.PackBuffer(m)
	if err != nil {
		return 0, err
	}
	defer pool.ReleaseBuf(buf)

	n, err = c.WriteTo(mRaw, to)
	if err != nil {
		err = WrapIOErr(err)
	}
	return
}

// ReadRawMsgFromTCP reads msg from c in RFC 7766 format.
// n represents how many bytes are read from c.
// This includes two-octet length field.
func ReadRawMsgFromTCP(c io.Reader) ([]byte, int, error) {
	n := 0
	lengthRaw := make([]byte, 2)
	nh, err := io.ReadFull(c, lengthRaw)
	n += nh
	if err != nil {
		return nil, n, err
	}

	// dns length
	length := binary.BigEndian.Uint16(lengthRaw)

	buf := pool.GetBuf(int(length))
	defer pool.ReleaseBuf(buf)

	nm, err := io.ReadFull(c, buf)
	n += nm
	if err != nil {
		return nil, n, err
	}

	m := make([]byte, nm)
	copy(m, buf)
	return m, n, nil
}

// ReadMsgFromTCP reads msg from c in RFC 7766 format.
// n represents how many bytes are read from c.
// This includes two-octet length field.
// An io err will be wrapped into an IOErr.
// IsIOErr(err) can check and unwrap the inner io err.
func ReadMsgFromTCP(c io.Reader) (*dns.Msg, int, error) {
	n := 0
	lengthRaw := make([]byte, 2)
	nh, err := io.ReadFull(c, lengthRaw)
	n += nh
	if err != nil {
		err = WrapIOErr(err)
		return nil, n, err
	}

	// dns length
	length := binary.BigEndian.Uint16(lengthRaw)

	buf := pool.GetBuf(int(length))
	defer pool.ReleaseBuf(buf)

	nm, err := io.ReadFull(c, buf)
	n += nm
	if err != nil {
		err = WrapIOErr(err)
		return nil, n, err
	}

	m := new(dns.Msg)
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
	defer pool.ReleaseBuf(buf)

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

	wb := pool.GetBuf(2 + len(b))
	defer pool.ReleaseBuf(wb)
	binary.BigEndian.PutUint16(wb[:2], uint16(len(b)))
	copy(wb[2:], b)
	n, err = c.Write(wb)
	if err != nil {
		err = WrapIOErr(err)
	}
	return
}
