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

	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type UDPServerOpts struct {
	Logger *zap.Logger
}

// ServeUDP starts a server at c. It returns if c had a read error.
// It always returns a non-nil error.
// h is required. logger is optional.
func ServeUDP(c *net.UDPConn, h Handler, opts UDPServerOpts) error {
	logger := opts.Logger
	if logger == nil {
		logger = nopLogger
	}

	listenerCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(errListenerCtxCanceled)

	rb := pool.GetBuf(dns.MaxMsgSize)
	defer pool.ReleaseBuf(rb)

	oobReader, oobWriter, err := initOobHandler(c)
	if err != nil {
		return fmt.Errorf("failed to init oob handler, %w", err)
	}
	var ob []byte
	if oobReader != nil {
		obp := pool.GetBuf(1024)
		defer pool.ReleaseBuf(obp)
		ob = *obp
	}

	for {
		n, oobn, _, remoteAddr, err := c.ReadMsgUDPAddrPort(*rb, ob)
		if err != nil {
			if n == 0 {
				// Err with zero read. Most likely because c was closed.
				return fmt.Errorf("unexpected read err: %w", err)
			}
			// Temporary err.
			logger.Warn("read err", zap.Error(err))
			continue
		}

		q := new(dns.Msg)
		if err := q.Unpack((*rb)[:n]); err != nil {
			logger.Warn("invalid msg", zap.Error(err), zap.Binary("msg", (*rb)[:n]), zap.Stringer("from", remoteAddr))
			continue
		}

		var dstIpFromCm net.IP
		if oobReader != nil {
			var err error
			dstIpFromCm, err = oobReader(ob[:oobn])
			if err != nil {
				logger.Error("failed to get dst address from oob", zap.Error(err))
			}
		}

		// handle query
		go func() {
			payload := h.Handle(listenerCtx, q, QueryMeta{ClientAddr: remoteAddr.Addr(), FromUDP: true}, pool.PackBuffer)
			if payload == nil {
				return
			}
			defer pool.ReleaseBuf(payload)

			var oob []byte
			if oobWriter != nil && dstIpFromCm != nil {
				oob = oobWriter(dstIpFromCm)
			}
			if _, _, err := c.WriteMsgUDPAddrPort(*payload, oob, remoteAddr); err != nil {
				logger.Warn("failed to write response", zap.Stringer("client", remoteAddr), zap.Error(err))
			}
		}()
	}
}

type getSrcAddrFromOOB func(oob []byte) (net.IP, error)
type writeSrcAddrToOOB func(a net.IP) []byte
