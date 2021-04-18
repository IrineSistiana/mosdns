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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
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

func (u *udpResponseWriter) Write(m *dns.Msg) (n int, err error) {
	m.Truncate(u.maxSize)
	return dnsutils.WriteUDPMsgTo(m, u.c, u.to)
}

// startUDP always returns a non-nil error.
func (s *Server) startUDP() error {
	c := s.packetConn

	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		q, from, _, err := dnsutils.ReadUDPMsgFrom(c, dnsutils.IPv4UdpMaxPayload)
		if err != nil {
			if ioErr := dnsutils.IsIOErr(err); ioErr != nil {
				if netErr, ok := ioErr.(net.Error); ok && netErr.Temporary() { // is a temporary net err
					s.logger.Warn("listener temporary err", zap.Error(err))
					time.Sleep(time.Second * 5)
					continue
				} else { // unexpected io err
					return fmt.Errorf("unexpected listener err: %w", err)
				}
			} else { // not an io err, maybe caused by an invalid msg.
				s.logger.Warn("udp read err", zap.Error(err))
				continue
			}
		}

		go func() {
			w := &udpResponseWriter{
				c:       c,
				to:      from,
				maxSize: getMaxSizeFromQuery(q),
			}
			qCtx := handler.NewContext(q, from)
			s.handleQuery(listenerCtx, qCtx, w)
		}()
	}
}
