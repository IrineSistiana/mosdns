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

	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/server/dns_handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type UDPServer struct {
	opts UDPServerOpts
}

func NewUDPServer(opts UDPServerOpts) *UDPServer {
	opts.init()
	return &UDPServer{opts: opts}
}

type UDPServerOpts struct {
	DNSHandler dns_handler.Handler // Required.
	Logger     *zap.Logger
}

func (opts *UDPServerOpts) init() {
	if opts.Logger == nil {
		opts.Logger = mlog.Nop()
	}
	return
}

// ServeUDP starts a server at c. It returns if c had a read error.
// It always returns a non-nil error.
func (s *UDPServer) ServeUDP(c *net.UDPConn) error {
	listenerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
			return fmt.Errorf("unexpected read err: %w", err)
		}
		clientAddr := remoteAddr.Addr()

		q := new(dns.Msg)
		if err := q.Unpack((*rb)[:n]); err != nil {
			s.opts.Logger.Warn("invalid msg", zap.Error(err), zap.Binary("msg", (*rb)[:n]), zap.Stringer("from", remoteAddr))
			continue
		}

		var dstIpFromCm net.IP
		if oobReader != nil {
			var err error
			dstIpFromCm, err = oobReader(ob[:oobn])
			if err != nil {
				s.opts.Logger.Error("failed to get dst address from oob", zap.Error(err))
			}
		}

		// handle query
		go func() {
			qCtx := query_context.NewContext(q)
			query_context.SetClientAddr(qCtx, &clientAddr)
			if err := s.opts.DNSHandler.ServeDNS(listenerCtx, qCtx); err != nil {
				s.opts.Logger.Warn("handler err", zap.Error(err))
				return
			}
			r := qCtx.R()
			if r != nil {
				r.Truncate(getUDPSize(q))
				b, buf, err := pool.PackBuffer(r)
				if err != nil {
					s.opts.Logger.Error("failed to unpack handler's response", zap.Error(err), zap.Stringer("msg", r))
					return
				}
				defer pool.ReleaseBuf(buf)
				var oob []byte

				if oobWriter != nil && dstIpFromCm != nil {
					oob = oobWriter(dstIpFromCm)
				}
				if _, _, err := c.WriteMsgUDPAddrPort(b, oob, remoteAddr); err != nil {
					s.opts.Logger.Warn("failed to write response", zap.Stringer("client", remoteAddr), zap.Error(err))
				}
			}
		}()
	}
}

func getUDPSize(m *dns.Msg) int {
	var s uint16
	if opt := m.IsEdns0(); opt != nil {
		s = opt.UDPSize()
	}
	if s < dns.MinMsgSize {
		s = dns.MinMsgSize
	}
	return int(s)
}

type getSrcAddrFromOOB func(oob []byte) (net.IP, error)
type writeSrcAddrToOOB func(a net.IP) []byte
