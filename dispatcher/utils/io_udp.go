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
	"github.com/miekg/dns"
	"io"
	"net"
)

const (
	IPv4UdpMaxPayload = 1472 // MTU 1500 - 20 IPv4 header - 8 udp header
	IPv6UdpMaxPayload = 1452 // MTU 1500 - 40 IPv6 header - 8 udp header
)

func ReadUDPMsgFrom(c net.PacketConn, bufSize int) (m *dns.Msg, from net.Addr, n int, err error) {
	buf := GetMsgBuf(bufSize)
	defer ReleaseMsgBuf(buf)

	n, from, err = c.ReadFrom(buf)
	if err != nil {
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

func ReadMsgFromUDP(c io.Reader, bufSize int) (m *dns.Msg, n int, err error) {
	buf := GetMsgBuf(bufSize)
	defer ReleaseMsgBuf(buf)

	n, err = c.Read(buf)
	if err != nil {
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

func WriteMsgToUDP(c io.Writer, m *dns.Msg) (n int, err error) {
	mRaw, buf, err := packMsgWithBuffer(m)
	if err != nil {
		return 0, err
	}
	defer ReleaseMsgBuf(buf)

	return WriteRawMsgToUDP(c, mRaw)
}

func WriteRawMsgToUDP(c io.Writer, b []byte) (n int, err error) {
	return c.Write(b)
}

func WriteUDPMsgTo(m *dns.Msg, c net.PacketConn, to net.Addr) (n int, err error) {
	mRaw, buf, err := packMsgWithBuffer(m)
	if err != nil {
		return 0, err
	}
	defer ReleaseMsgBuf(buf)

	return c.WriteTo(mRaw, to)
}
