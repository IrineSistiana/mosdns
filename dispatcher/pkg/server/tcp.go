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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server/dns_handler"
	"go.uber.org/zap"
	"io"
	"net"
	"time"
)

const (
	serverTCPWriteTimeout = time.Second
	defaultTCPIdleTimeout = time.Second * 10
)

type tcpResponseWriter struct {
	c net.Conn
}

func (t *tcpResponseWriter) Write(m []byte) (n int, err error) {
	t.c.SetWriteDeadline(time.Now().Add(serverTCPWriteTimeout))
	return dnsutils.WriteRawMsgToTCP(t.c, m)
}

func (s *Server) ServeTCP(l net.Listener) error {
	ol := &onceCloseListener{Listener: l}
	defer ol.Close()

	closer := io.Closer(ol)
	if ok := s.trackCloser(&closer, true); !ok {
		return ErrServerClosed
	}
	defer s.trackCloser(&closer, false)

	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		c, err := ol.Accept()
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Temporary() {
				s.getLogger().Warn("listener temporary err", zap.Error(err))
				time.Sleep(time.Second * 5)
				continue
			} else {
				if s.Closed() {
					return ErrServerClosed
				}
				return fmt.Errorf("unexpected listener err: %w", err)
			}
		}

		tcpConnCtx, cancelConn := context.WithCancel(listenerCtx)
		go func() {
			defer c.Close()
			defer cancelConn()

			for {
				c.SetReadDeadline(time.Now().Add(s.getIdleTimeout()))
				req, _, err := dnsutils.ReadRawMsgFromTCP(c)
				if err != nil {
					return // read err, close the connection
				}

				go func() {
					s.DNSHandler.ServeDNS(tcpConnCtx, &dns_handler.Request{Msg: req, From: c.RemoteAddr()}, &tcpResponseWriter{c: c})
				}()
			}
		}()
	}
}
