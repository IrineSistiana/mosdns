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
	"errors"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"testing"
	"time"
)

const (
	defaultQueryTimeout = time.Second * 5
)

var (
	nopLogger = zap.NewNop()
)

// Handler handles dns query.
type Handler interface {
	// ServeDNS handles incoming request req and returns a response.
	// Implements must not keep and use req after the ServeDNS returned.
	// ServeDNS should handle dns errors by itself and return a proper error responses
	// for clients.
	// ServeDNS should always return a responses.
	// If ServeDNS returns an error, caller considers that the error is associated
	// with the downstream connection and will close the downstream connection
	// immediately.
	// All input parameters won't be nil.
	ServeDNS(ctx context.Context, req *dns.Msg, meta *query_context.RequestMeta) (*dns.Msg, error)
}

type EntryHandlerOpts struct {
	// Logger is used for logging. Default is a noop logger.
	Logger *zap.Logger

	Entry executable_seq.Executable

	// QueryTimeout limits the timeout value of each query.
	// Default is defaultQueryTimeout.
	QueryTimeout time.Duration

	// RecursionAvailable sets the dns.Msg.RecursionAvailable flag globally.
	RecursionAvailable bool
}

func (opts *EntryHandlerOpts) Init() error {
	if opts.Logger == nil {
		opts.Logger = nopLogger
	}
	if opts.Entry == nil {
		return errors.New("nil entry")
	}
	utils.SetDefaultNum(&opts.QueryTimeout, defaultQueryTimeout)
	return nil
}

type EntryHandler struct {
	opts EntryHandlerOpts
}

func NewEntryHandler(opts EntryHandlerOpts) (*EntryHandler, error) {
	if err := opts.Init(); err!= nil {
		return nil, err
	}
	return &EntryHandler{opts: opts}, nil
}

// ServeDNS implements Handler.
// If entry returns an error, a SERVFAIL response will be returned.
// If entry returns without a response, a REFUSED response will be returned.
func (h *EntryHandler) ServeDNS(ctx context.Context, req *dns.Msg, meta *query_context.RequestMeta) (*dns.Msg, error) {
	// apply timeout to ctx
	ddl := time.Now().Add(h.opts.QueryTimeout)
	ctxDdl, ok := ctx.Deadline()
	if!(ok && ctxDdl.Before(ddl)) {
		newCtx, cancel := context.WithDeadline(ctx, ddl)
		defer cancel()
		ctx = newCtx
	}

	// exec entry
	qCtx := query_context.NewContext(req, meta)
	err := h.opts.Entry.Exec(ctx, qCtx, nil)
	respMsg := qCtx.R()
	if err!= nil {
		h.opts.Logger.Warn("entry returned an err", qCtx.InfoField(), zap.Error(err))
		respMsg = new(dns.Msg)
		respMsg.SetReply(req)
		respMsg.Rcode = dns.RcodeServerFailure
	} else {
		h.opts.Logger.Debug("entry returned", qCtx.InfoField())
		if respMsg == nil {
			h.opts.Logger.Error("entry returned an nil response", qCtx.InfoField())
			respMsg = new(dns.Msg)
			respMsg.SetReply(req)
			respMsg.Rcode = dns.RcodeRefused
		}
	}

	if h.opts.RecursionAvailable {
		respMsg.RecursionAvailable = true
	}
	return respMsg, nil
}

type DummyServerHandler struct {
	T       *testing.T
	WantMsg *dns.Msg
	WantErr error
}

func (d *DummyServerHandler) ServeDNS(_ context.Context, req *dns.Msg, meta *query_context.RequestMeta) (*dns.Msg, error) {
	if d.WantErr!= nil {
		return nil, d.WantErr
	}

	var resp *dns.Msg
	if d.WantMsg!= nil {
		resp = d.WantMsg.Copy()
		resp.Id = req.Id
	} else {
		resp = new(dns.Msg)
		resp.SetReply(req)
	}
	return resp, nil
}

// Optimization: Add a pool for dns.Msg to reduce memory allocation and garbage collection.
var msgPool = &sync.Pool{
	New: func() interface{} {
		return new(dns.Msg)
	},
}

func getMsgFromPool() *dns.Msg {
	return msgPool.Get().(*dns.Msg)
}

func putMsgToPool(msg *dns.Msg) {
	msg.Id = 0
	msg.Question = nil
	msg.Answer = nil
	msg.Ns = nil
	msg.Extra = nil
	msgPool.Put(msg)
}
