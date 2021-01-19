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
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
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
	return utils.WriteUDPMsgTo(m, u.c, u.to)
}

func (sg *ServerGroup) startUDP(conf *ServerConfig) error {
	c, err := net.ListenPacket("udp", conf.Addr)
	if err != nil {
		return err
	}
	sg.listener[c] = struct{}{}
	sg.L().Info("udp server started", zap.Stringer("addr", c.LocalAddr()))
	go func() {
		listenerCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for {
			q, from, _, err := utils.ReadUDPMsgFrom(c, utils.IPv4UdpMaxPayload)
			if err != nil {
				if sg.isClosed() {
					return
				}
				netErr, ok := err.(net.Error)
				if ok { // is a net err
					if netErr.Temporary() {
						sg.L().Warn("listener temporary err", zap.Stringer("addr", c.LocalAddr()), zap.Error(err))
						time.Sleep(time.Second * 5)
						continue
					} else {
						sg.errChan <- fmt.Errorf("unexpected listener err: %w", err)
						return
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
			qCtx := handler.NewContext(q, from)
			sg.L().Debug("new query", qCtx.InfoField(), zap.Stringer("from", from))

			go func() {
				queryCtx, cancel := context.WithTimeout(listenerCtx, time.Second*5)
				defer cancel()
				sg.handler.ServeDNS(queryCtx, qCtx, w)
			}()
		}
	}()
	return nil
}
