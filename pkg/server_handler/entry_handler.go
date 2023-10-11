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
)

var (
	nopLogger = mlog.Nop()

	// options that can forward to upstream
	queryForwardEDNS0Option = map[uint16]struct{}{
		dns.EDNS0SUBNET: {},
	}

	// options that useless for downstream
	respRemoveEDNS0Option = map[uint16]struct{}{
		dns.EDNS0PADDING: {},
	}
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
func (h *EntryHandler) Handle(ctx context.Context, q *dns.Msg, serverMeta server.QueryMeta, packMsgPayload func(m *dns.Msg) (*[]byte, error)) *[]byte {
	// basic query check.
	if q.Response || len(q.Question) != 1 || len(q.Answer)+len(q.Ns) > 0 || len(q.Extra) > 1 {
		return nil
	}

	ddl := time.Now().Add(h.opts.QueryTimeout)
	ctx, cancel := context.WithDeadline(ctx, ddl)
	defer cancel()

	qCtx := query_context.NewContext(q)
	qCtx.ServerMeta = serverMeta

	// exec entry
	err := h.opts.Entry.Exec(ctx, qCtx)
	var resp *dns.Msg
	if err != nil {
		h.opts.Logger.Warn("entry err", qCtx.InfoField(), zap.Error(err))
		resp = new(dns.Msg)
		resp.SetReply(q)
		resp.Rcode = dns.RcodeServerFailure
	} else {
		resp = qCtx.R()
	}

	if resp == nil {
		resp = new(dns.Msg)
		resp.SetReply(q)
		resp.Rcode = dns.RcodeRefused
	}
	// We assume that our server is a forwarder.
	resp.RecursionAvailable = true

	// add respOpt back to resp
	if respOpt := qCtx.RespOpt(); respOpt != nil {
		resp.Extra = append(resp.Extra, respOpt)
	}

	if serverMeta.FromUDP {
		udpSize := getValidUDPSize(qCtx.ClientOpt())
		resp.Truncate(udpSize)
	}

	payload, err := packMsgPayload(resp)
	if err != nil {
		h.opts.Logger.Error("internal err: failed to pack resp msg", qCtx.InfoField(), zap.Error(err))
		return nil
	}
	return payload
}

// opt can be nil.
func getValidUDPSize(opt *dns.OPT) int {
	var s uint16
	if opt != nil {
		s = opt.UDPSize()
	}
	if s < dns.MinMsgSize {
		s = dns.MinMsgSize
	}
	return int(s)
}

func newOpt() *dns.OPT {
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	return opt
}
