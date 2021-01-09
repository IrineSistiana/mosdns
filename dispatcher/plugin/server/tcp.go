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
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
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
	return utils.WriteMsgToTCP(t.c, m)
}

func (s *server) startTCP(conf *ServerConfig, isDoT bool) error {
	l, err := net.Listen("tcp", conf.Addr)
	if err != nil {
		return err
	}
	s.listener[l] = struct{}{}
	s.L().Info("tcp server started", zap.Stringer("addr", l.Addr()))

	if isDoT {
		if len(conf.Cert) == 0 || len(conf.Key) == 0 {
			return errors.New("dot server needs cert and key")
		}
		tlsConfig := new(tls.Config)
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(conf.Cert, conf.Key)
		if err != nil {
			return err
		}

		l = tls.NewListener(l, tlsConfig)
	}

	go func() {
		listenerCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		for {
			c, err := l.Accept()
			if err != nil {
				if s.isClosed() {
					return
				}

				netErr, ok := err.(net.Error)
				if ok && netErr.Temporary() {
					s.L().Warn("listener temporary err", zap.Stringer("addr", l.Addr()), zap.Error(err))
					time.Sleep(time.Second * 5)
					continue
				} else {
					s.errChan <- fmt.Errorf("unexpected listener err: %w", err)
					return
				}
			}

			tcpConnCtx, cancelConn := context.WithCancel(listenerCtx)
			go func() {
				defer c.Close()
				defer cancelConn()

				for {
					c.SetReadDeadline(time.Now().Add(conf.idleTimeout))
					q, _, err := utils.ReadMsgFromTCP(c)
					if err != nil {
						return // read err, close the conn
					}

					w := &tcpResponseWriter{c: c}

					qCtx := handler.NewContext(q, c.RemoteAddr())
					s.L().Debug("new query", qCtx.InfoField(), zap.Stringer("from", c.RemoteAddr()))

					ctx, cancelQuery := context.WithTimeout(tcpConnCtx, conf.queryTimeout)
					go func() {
						defer cancelQuery()
						s.handler.ServeDNS(ctx, qCtx, w)
					}()
				}
			}()
		}
	}()

	return nil
}
