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
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
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
	return utils.WriteMsgToTCP(t.c, m)
}

func (s *Server) serveTCP(isDoT bool) error {
	l, err := net.Listen("tcp", s.Config.Addr)
	if err != nil {
		return err
	}

	s.l.Lock()
	s.listener = l
	s.l.Unlock()
	defer func() {
		s.l.Lock()
		l.Close()
		s.l.Unlock()
	}()

	if isDoT {
		if len(s.Config.Cert) == 0 || len(s.Config.Key) == 0 {
			return errors.New("dot server needs cert and key")
		}
		tlsConfig := new(tls.Config)
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(s.Config.Cert, s.Config.Key)
		if err != nil {
			return err
		}

		l = tls.NewListener(l, tlsConfig)
	}

	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		c, err := l.Accept()
		if err != nil {
			if s.isDone() {
				return nil
			}

			netErr, ok := err.(net.Error)
			if ok && netErr.Temporary() {
				s.Logger.Warnf("listener: temporary err: %v", err)
				time.Sleep(time.Second * 5)
				continue
			} else {
				return fmt.Errorf("unexpected listener err: %w", err)
			}
		}
		tcpConnCtx, cancel := context.WithCancel(listenerCtx)

		go func() {
			defer c.Close()
			defer cancel()

			for {
				c.SetReadDeadline(time.Now().Add(s.idleTimeout))
				q, _, err := utils.ReadMsgFromTCP(c)
				if err != nil {
					return // read err, close the conn
				}

				w := &tcpResponseWriter{c: c}

				qCtx := handler.NewContext(q)
				qCtx.From = c.RemoteAddr()

				ctx, cancel := context.WithTimeout(tcpConnCtx, s.queryTimeout)
				go func() {
					defer cancel()
					s.Handler.ServeDNS(ctx, qCtx, w)
				}()
			}
		}()
	}
}
