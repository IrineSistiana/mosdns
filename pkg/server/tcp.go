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
	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"go.uber.org/zap"
	"net"
	"time"
)

const (
	defaultTCPIdleTimeout = time.Second * 10
	tcpFirstReadTimeout   = time.Millisecond * 500
)

type TCPServer struct {
	opts TCPServerOpts
}

func NewTCPServer(opts TCPServerOpts) *TCPServer {
	opts.init()
	return &TCPServer{opts: opts}
}

type TCPServerOpts struct {
	DNSHandler  dns_handler.Handler // Required.
	Logger      *zap.Logger
	IdleTimeout time.Duration
}

func (opts *TCPServerOpts) init() {
	if opts.Logger == nil {
		opts.Logger = mlog.Nop()
	}
	utils.SetDefaultNum(&opts.IdleTimeout, defaultTCPIdleTimeout)
	return
}

// ServeTCP starts a server at l. It returns if l had an Accept() error.
// It always returns a non-nil error.
func (s *TCPServer) ServeTCP(l net.Listener) error {
	// handle listener
	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		c, err := l.Accept()
		if err != nil {
			return fmt.Errorf("unexpected listener err: %w", err)
		}

		// handle connection
		tcpConnCtx, cancelConn := context.WithCancel(listenerCtx)
		go func() {
			defer c.Close()
			defer cancelConn()

			firstReadTimeout := tcpFirstReadTimeout
			idleTimeout := s.opts.IdleTimeout
			if idleTimeout < firstReadTimeout {
				firstReadTimeout = idleTimeout
			}

			clientAddr := utils.GetAddrFromAddr(c.RemoteAddr())

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
					qCtx := query_context.NewContext(req)
					query_context.SetClientAddr(qCtx, &clientAddr)
					if err := s.opts.DNSHandler.ServeDNS(tcpConnCtx, qCtx); err != nil {
						s.opts.Logger.Warn("handler err", zap.Error(err))
						c.Close()
						return
					}
					r := qCtx.R()

					b, buf, err := pool.PackBuffer(r)
					if err != nil {
						s.opts.Logger.Error("failed to unpack handler's response", zap.Error(err), zap.Stringer("msg", r))
						return
					}
					defer pool.ReleaseBuf(buf)

					if _, err := dnsutils.WriteRawMsgToTCP(c, b); err != nil {
						s.opts.Logger.Warn("failed to write response", zap.Stringer("client", c.RemoteAddr()), zap.Error(err))
						return
					}
				}()
			}
		}()
	}
}
