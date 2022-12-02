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

package dns_handler

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"time"
)

const (
	defaultQueryTimeout = time.Second * 5
)

var (
	nopLogger = mlog.Nop()
)

// Handler handles dns query.
type Handler interface {
	// ServeDNS handles incoming request qCtx and MUST ALWAYS set a response.
	// Implements must not keep and use qCtx after the ServeDNS returned.
	// ServeDNS should handle dns errors by itself and return a proper error responses
	// for clients.
	// If ServeDNS returns an error, caller considers that the error is associated
	// with the downstream connection and will close the downstream connection
	// immediately.
	ServeDNS(ctx context.Context, qCtx *query_context.Context) error
}

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

func NewEntryHandler(opts EntryHandlerOpts) *EntryHandler {
	opts.init()
	return &EntryHandler{opts: opts}
}

// ServeDNS implements Handler.
// If entry returns an error, a SERVFAIL response will be set.
// If entry returns without a response, a REFUSED response will be set.
func (h *EntryHandler) ServeDNS(ctx context.Context, qCtx *query_context.Context) error {
	ddl := time.Now().Add(h.opts.QueryTimeout)
	ctx, cancel := context.WithDeadline(ctx, ddl)
	defer cancel()

	// exec entry
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
	qCtx.SetResponse(respMsg)
	return nil
}
