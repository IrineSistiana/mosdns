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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"time"
)

type udpResponseWriter struct {
	c       net.PacketConn
	to      net.Addr
	maxSize int
}

func getMaxSizeFromQuery(m *dns.Msg) int {
	if opt := m.IsEdns0(); opt != nil && opt.Hdr.Class > dns.MinMsgSize {
		return int(opt.Hdr.Class)
	} else {
		return dns.MinMsgSize
	}
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

	for {
		m, from, _, err := dnsutils.ReadRawUDPMsgFrom(c, 4096)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() { // is a temporary net err
				s.getLogger().Warn("listener temporary err", zap.Error(err))
				time.Sleep(time.Second * 5)
				continue
			} else { // unexpected io err
				if s.Closed() {
					return ErrServerClosed
				}
				return fmt.Errorf("unexpected listener err: %w", err)
			}
		}

		go func() {
			w := &udpResponseWriter{
				c:  ol,
				to: from,
			}

			var meta *handler.RequestMeta
			if clientIP := utils.GetIPFromAddr(from); clientIP != nil {
				meta = &handler.RequestMeta{ClientIP: clientIP}
			} else {
				s.getLogger().Warn("failed to acquire client ip addr")
			}
			s.DNSHandler.ServeDNS(listenerCtx, m, w, meta) // meta maybe nil
		}()
	}
}
