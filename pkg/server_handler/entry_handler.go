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

package server_handler

import (
	"context"
	"time"

	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/server"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const (
	defaultQueryTimeout = time.Second * 5
	edns0Size           = 1200
)

var (
	nopLogger = mlog.Nop()
)

type EntryHandlerOpts struct {
	// Logger is used for logging. Default is a noop logger.
	Logger *zap.Logger

	// Required.
	Entry sequence.Executable

	// QueryTimeout limits the timeout value of each query.
	// Default is defaultQueryTimeout.
	QueryTimeout time.Duration
}

func (opts *EntryHandlerOpts) init() {
	if opts.Logger == nil {
		opts.Logger = nopLogger
	}
	utils.SetDefaultNum(&opts.QueryTimeout, defaultQueryTimeout)
}

type EntryHandler struct {
	opts EntryHandlerOpts
}

var _ server.Handler = (*EntryHandler)(nil)

func NewEntryHandler(opts EntryHandlerOpts) *EntryHandler {
	opts.init()
	return &EntryHandler{opts: opts}
}

// ServeDNS implements server.Handler.
// If entry returns an error, a SERVFAIL response will be returned.
// If entry returns without a response, a REFUSED response will be returned.
func (h *EntryHandler) Handle(ctx context.Context, q *dns.Msg, qInfo server.QueryMeta, packMsgPayload func(m *dns.Msg) (*[]byte, error)) *[]byte {
	ddl := time.Now().Add(h.opts.QueryTimeout)
	ctx, cancel := context.WithDeadline(ctx, ddl)
	defer cancel()

	// Get udp size before exec plugins. It may be changed by plugins.
	queryUdpSize := getUDPSize(q)

	// Enable edns0. We can handle this. 
	// This also helps to avoid udp->tcp fallback.
	ends0Upgraded := false
	if opt := q.IsEdns0(); opt == nil {
		q.SetEdns0(edns0Size, false)
		ends0Upgraded = true
	} else {
		opt.SetUDPSize(edns0Size)
	}

	// exec entry
	qCtx := query_context.NewContext(q, qInfo)
	err := h.opts.Entry.Exec(ctx, qCtx)
	respMsg := qCtx.R()
	if err != nil {
		h.opts.Logger.Warn("entry err", qCtx.InfoField(), zap.Error(err))
	}

	if err == nil && respMsg == nil {
		respMsg = new(dns.Msg)
		respMsg.SetReply(qCtx.Q())
		respMsg.Rcode = dns.RcodeRefused
	}
	if err != nil {
		respMsg = new(dns.Msg)
		respMsg.SetReply(qCtx.Q())
		respMsg.Rcode = dns.RcodeServerFailure
	}
	respMsg.RecursionAvailable = true

	// Client may not support edns0.
	// Remove edns0 from resp, as RFC 2671 required.
	if ends0Upgraded {
		dnsutils.RemoveEDNS0(respMsg)
	}

	if qInfo.FromUDP {
		respMsg.Truncate(queryUdpSize)
	}

	payload, err := packMsgPayload(respMsg)
	if err != nil {
		h.opts.Logger.Error("internal err: failed to pack resp msg", zap.Error(err))
		return nil
	}
	return payload
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
