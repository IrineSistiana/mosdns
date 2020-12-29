//     Copyright (C) 2020, IrineSistiana
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
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
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
	return utils.WriteUDPMsgTo(m, u.c, u.to)
}

func (s *Server) serveUDP() error {
	c, err := net.ListenPacket("udp", s.Config.Addr)
	if err != nil {
		return err
	}

	s.l.Lock()
	s.packetConn = c
	s.l.Unlock()
	defer func() {
		s.l.Lock()
		c.Close()
		s.l.Unlock()
	}()

	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		q, from, _, err := utils.ReadUDPMsgFrom(c, utils.IPv4UdpMaxPayload)
		if err != nil {
			if s.isDone() {
				return nil
			}
			netErr, ok := err.(net.Error)
			if ok { // is a net err
				if netErr.Temporary() {
					s.Logger.Warnf("listener temporary err: %v", err)
					time.Sleep(time.Second * 5)
					continue
				} else {
					return fmt.Errorf("unexpected listener err: %w", err)
				}
			} else { // invalid msg
				continue
			}
		}
		w := &udpResponseWriter{
			c:       c,
			to:      from,
			maxSize: getMaxSizeFromQuery(q),
		}
		qCtx := handler.NewContext(q)
		qCtx.From = from

		go func() {
			queryCtx, cancel := context.WithTimeout(listenerCtx, time.Second*5)
			defer cancel()
			s.Handler.ServeDNS(queryCtx, qCtx, w)
		}()
	}
}
