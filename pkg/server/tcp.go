/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package server

import (
	"context"
	"fmt"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/dnsutils"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/pool"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/utils"
	"go.uber.org/zap"
	"io"
	"net"
	"time"
)

const (
	defaultTCPIdleTimeout = time.Second * 10
	tcpFirstReadTimeout   = time.Millisecond * 500
)

func (s *Server) ServeTCP(l net.Listener) error {
	defer l.Close()

	handler := s.opts.DNSHandler
	if handler == nil {
		return errMissingDNSHandler
	}

	closer := l.(io.Closer)
	if ok := s.trackCloser(&closer, true); !ok {
		return ErrServerClosed
	}
	defer s.trackCloser(&closer, false)

	// handle listener
	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		c, err := l.Accept()
		if err != nil {
			if s.Closed() {
				return ErrServerClosed
			}
			return fmt.Errorf("unexpected listener err: %w", err)
		}

		// handle connection
		tcpConnCtx, cancelConn := context.WithCancel(listenerCtx)
		go func() {
			defer c.Close()
			defer cancelConn()

			closer := c.(io.Closer)
			if !s.trackCloser(&closer, true) {
				return
			}
			defer s.trackCloser(&closer, false)

			firstReadTimeout := tcpFirstReadTimeout
			idleTimeout := s.opts.IdleTimeout
			if idleTimeout < firstReadTimeout {
				firstReadTimeout = idleTimeout
			}

			clientAddr := utils.GetAddrFromAddr(c.RemoteAddr())
			meta := &query_context.RequestMeta{
				ClientAddr: clientAddr,
			}

			firstRead := true
			for {
				if firstRead {
					firstRead = false
					c.SetReadDeadline(time.Now().Add(firstReadTimeout))
				} else {
					c.SetReadDeadline(time.Now().Add(idleTimeout))
				}
				req, _, err := dnsutils.ReadMsgFromTCP(c)
				if err != nil {
					return // read err, close the connection
				}

				// handle query
				go func() {
					r, err := handler.ServeDNS(tcpConnCtx, req, meta)
					if err != nil {
						s.opts.Logger.Warn("handler err", zap.Error(err))
						c.Close()
						return
					}

					b, buf, err := pool.PackBuffer(r)
					if err != nil {
						s.opts.Logger.Error("failed to unpack handler's response", zap.Error(err), zap.Stringer("msg", r))
						return
					}
					defer buf.Release()

					if _, err := dnsutils.WriteRawMsgToTCP(c, b); err != nil {
						s.opts.Logger.Warn("failed to write response", zap.Stringer("client", c.RemoteAddr()), zap.Error(err))
						return
					}
				}()
			}
		}()
	}
}
