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

package plainserver

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
	c  net.PacketConn
	to net.Addr
}

func (u *udpResponseWriter) Write(m *dns.Msg) (n int, err error) {
	var maxSize int
	if opt := m.IsEdns0(); opt != nil {
		maxSize = int(opt.Hdr.Class)
	} else {
		maxSize = dns.MinMsgSize
	}

	m.Truncate(maxSize)
	return utils.WriteUDPMsgTo(m, u.c, u.to)
}

func (s *singleServer) serveUDP(c net.PacketConn, h handler.ServerHandler) error {
	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		q, from, _, err := utils.ReadUDPMsgFrom(c, utils.IPv4UdpMaxPayload)
		if err != nil {
			select {
			case <-s.shutdownChan:
				return nil
			default:
			}
			netErr, ok := err.(net.Error)
			if ok { // is a net err
				if netErr.Temporary() {
					s.logger.Warnf("udp server: listener temporary err: %v", err)
					time.Sleep(time.Second * 5)
					continue
				} else {
					return fmt.Errorf("udp server: unexpected listener err: %w", err)
				}
			} else { // invalid msg
				continue
			}
		}
		w := &udpResponseWriter{
			c:  c,
			to: from,
		}
		qCtx := &handler.Context{
			Q:    q,
			From: from,
		}
		go h.ServeDNS(listenerCtx, qCtx, w)
	}
}
