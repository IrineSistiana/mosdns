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
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"net"
	"time"
)

const (
	serverTCPWriteTimeout = time.Second
)

type tcpResponseWriter struct {
	c net.Conn
}

func (t *tcpResponseWriter) Write(m *dns.Msg) (n int, err error) {
	t.c.SetWriteDeadline(time.Now().Add(serverTCPWriteTimeout))
	return dnsutils.WriteMsgToTCP(t.c, m)
}

func (s *Server) startTCP() error {
	if s.Listener == nil {
		return errors.New("tcp server has a nil listener")
	}

	l := s.Listener
	isDoT := s.Protocol == ProtocolDoT
	if isDoT {
		if err := checkTLSConfig(s.TLSConfig); err != nil {
			return err
		}
		l = tls.NewListener(l, s.TLSConfig)
	}

	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		c, err := l.Accept()
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Temporary() {
				s.logger.Warn("listener temporary err", zap.Error(err))
				time.Sleep(time.Second * 5)
				continue
			} else {
				return fmt.Errorf("unexpected listener err: %w", err)
			}
		}

		tcpConnCtx, cancelConn := context.WithCancel(listenerCtx)
		go func() {
			defer c.Close()
			defer cancelConn()

			for {
				c.SetReadDeadline(time.Now().Add(s.idleTimeout()))
				q, _, err := dnsutils.ReadMsgFromTCP(c)
				if err != nil {
					return // read err, close the connection
				}

				go func() {
					qCtx := handler.NewContext(q, c.RemoteAddr())
					qCtx.SetTCPClient(true)
					s.handleQueryTimeout(tcpConnCtx, qCtx, &tcpResponseWriter{c: c})
				}()
			}
		}()
	}
}
