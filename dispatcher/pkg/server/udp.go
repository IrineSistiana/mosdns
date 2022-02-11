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

package server

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/pool"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"io"
	"net"
)

type udpResponseWriter struct {
	c  net.PacketConn
	to net.Addr
}

func (u *udpResponseWriter) Write(m []byte) (n int, err error) {
	return u.c.WriteTo(m, u.to)
}

func (s *Server) ServeUDP(c net.PacketConn) error {
	ol := &onceClosePackageConn{PacketConn: c}
	defer ol.Close()

	closer := io.Closer(ol)
	if ok := s.trackCloser(&closer, true); !ok {
		return ErrServerClosed
	}
	defer s.trackCloser(&closer, false)

	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	readBuf := pool.GetBuf(64 * 1024)
	defer readBuf.Release()
	rb := readBuf.Bytes()

	for {
		n, from, err := c.ReadFrom(rb)
		if err != nil {
			if s.Closed() {
				return ErrServerClosed
			}
			return fmt.Errorf("unexpected read err: %w", err)
		}

		reqBuf := pool.GetBuf(n)
		copy(reqBuf.Bytes(), rb[:n])

		go func() {
			defer reqBuf.Release()
			var meta *handler.RequestMeta
			if clientIP := utils.GetIPFromAddr(from); clientIP != nil {
				meta = &handler.RequestMeta{ClientIP: clientIP}
			} else {
				s.getLogger().Warn("failed to acquire client ip addr")
			}

			w := &udpResponseWriter{c: ol, to: from}
			s.DNSHandler.ServeDNS(listenerCtx, reqBuf.Bytes(), w, meta) // meta maybe nil
		}()
	}
}
