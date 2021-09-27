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
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/dnsutils"
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

// startTCP always returns a non-nil error.
func (s *Server) startTCP() error {
	return s.serveTCP(s.listener)
}

// startDoT always returns a non-nil error.
func (s *Server) startDoT() error {
	var tlsConfig *tls.Config
	if s.tlsConfig != nil {
		tlsConfig = s.tlsConfig.Clone()
	} else {
		tlsConfig = new(tls.Config)
	}

	if len(s.key)+len(s.cert) != 0 {
		cert, err := tls.LoadX509KeyPair(s.cert, s.key)
		if err != nil {
			return err
		}
		tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
	}

	return s.serveTCP(tls.NewListener(s.listener, tlsConfig))
}

func (s *Server) serveTCP(l net.Listener) error {
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
				c.SetReadDeadline(time.Now().Add(s.getIdleTimeout()))
				q, _, err := dnsutils.ReadMsgFromTCP(c)
				if err != nil {
					return // read err, close the connection
				}

				go func() {
					qCtx := handler.NewContext(q, c.RemoteAddr())
					s.handleQuery(tcpConnCtx, qCtx, &tcpResponseWriter{c: c})
				}()
			}
		}()
	}
}
