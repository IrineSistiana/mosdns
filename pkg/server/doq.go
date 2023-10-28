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
	"net"
	"net/netip"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

const (
	defaultQuicIdleTimeout = time.Second * 30
	streamReadTimeout      = time.Second * 2
	quicFirstReadTimeout   = time.Second * 2
)

type DoQServerOpts struct {
	Logger      *zap.Logger
	IdleTimeout time.Duration
}

// ServeDoQ starts a server at l. It returns if l had an Accept() error.
// It always returns a non-nil error.
func ServeDoQ(l *quic.Listener, h Handler, opts DoQServerOpts) error {
	logger := opts.Logger
	if logger == nil {
		logger = nopLogger
	}
	idleTimeout := opts.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultQuicIdleTimeout
	}

	listenerCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(errListenerCtxCanceled)
	for {
		c, err := l.Accept(listenerCtx)
		if err != nil {
			return fmt.Errorf("unexpected listener err: %w", err)
		}

		// handle connection
		connCtx, cancelConn := context.WithCancelCause(listenerCtx)
		go func() {
			defer c.CloseWithError(0, "")
			defer cancelConn(errConnectionCtxCanceled)

			var clientAddr netip.Addr
			ta, ok := c.RemoteAddr().(*net.UDPAddr)
			if ok {
				clientAddr = ta.AddrPort().Addr()
			}

			firstRead := true
			for {
				var streamAcceptTimeout time.Duration
				if firstRead {
					firstRead = false
					streamAcceptTimeout = quicFirstReadTimeout
				} else {
					streamAcceptTimeout = idleTimeout
				}
				streamAcceptCtx, cancelStreamAccept := context.WithTimeout(connCtx, streamAcceptTimeout)
				stream, err := c.AcceptStream(streamAcceptCtx)
				cancelStreamAccept()
				if err != nil {
					return
				}

				// Handle stream.
				// For doq, one stream, one query.
				go func() {
					defer func() {
						stream.Close()
						stream.CancelRead(0) // TODO: Needs a proper error code.
					}()
					// Avoid fragmentation attack.
					stream.SetReadDeadline(time.Now().Add(streamReadTimeout))
					req, _, err := dnsutils.ReadMsgFromTCP(stream)
					if err != nil {
						return
					}
					queryMeta := QueryMeta{
						ClientAddr: clientAddr,
						ServerName: c.ConnectionState().TLS.ServerName,
					}

					resp := h.Handle(connCtx, req, queryMeta, pool.PackTCPBuffer)
					if resp == nil {
						return
					}
					if _, err := stream.Write(*resp); err != nil {
						logger.Warn("failed to write response", zap.Stringer("client", c.RemoteAddr()), zap.Error(err))
					}
				}()
			}
		}()
	}
}
