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
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"go.uber.org/zap"
)

const (
	defaultTCPIdleTimeout = time.Second * 10
	tcpFirstReadTimeout   = time.Second * 2
)

type TCPServerOpts struct {
	// Nil logger == nop
	Logger *zap.Logger

	// Default is defaultTCPIdleTimeout.
	IdleTimeout time.Duration
}

// ServeTCP starts a server at l. It returns if l had an Accept() error.
// It always returns a non-nil error.
func ServeTCP(l net.Listener, h Handler, opts TCPServerOpts) error {
	logger := opts.Logger
	if logger == nil {
		logger = nopLogger
	}
	idleTimeout := opts.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultTCPIdleTimeout
	}
	firstReadTimeout := tcpFirstReadTimeout
	if idleTimeout < firstReadTimeout {
		firstReadTimeout = idleTimeout
	}

	listenerCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(errListenerCtxCanceled)
	for {
		c, err := l.Accept()
		if err != nil {
			return fmt.Errorf("unexpected listener err: %w", err)
		}

		// handle connection
		tcpConnCtx, cancelConn := context.WithCancelCause(listenerCtx)
		go func() {
			defer c.Close()
			defer cancelConn(errConnectionCtxCanceled)

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

				// Try to get server name from tls conn.
				var serverName string
				if tlsConn, ok := c.(*tls.Conn); ok {
					serverName = tlsConn.ConnectionState().ServerName
				}

				// handle query
				go func() {
					var clientAddr netip.Addr
					ta, ok := c.RemoteAddr().(*net.TCPAddr)
					if ok {
						clientAddr = ta.AddrPort().Addr()
					}
					r := h.Handle(tcpConnCtx, req, QueryMeta{ClientAddr: clientAddr, ServerName: serverName}, pool.PackTCPBuffer)
					if r == nil {
						c.Close() // abort the connection
						return
					}
					defer pool.ReleaseBuf(r)

					if _, err := c.Write(*r); err != nil {
						logger.Warn("failed to write response", zap.Stringer("client", c.RemoteAddr()), zap.Error(err))
						return
					}
				}()
			}
		}()
	}
}
