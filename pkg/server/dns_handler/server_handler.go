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
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"testing"
	"time"
)

const (
	defaultQueryTimeout = time.Second * 5
)

// Handler handles dns query.
type Handler interface {
	// ServeDNS handles r and writes response to w.
	// Implements must not keep and use req after the ServeDNS returned.
	// ServeDNS should handle errors by itself and sends properly error responses
	// to clients.
	// If ServeDNS returns an error, caller considers that the error is associated
	// with the downstream connection and will close the downstream connection
	// immediately.
	ServeDNS(ctx context.Context, req *dns.Msg, w ResponseWriter, meta *query_context.RequestMeta) error
}

// ResponseWriter can write msg to the client.
type ResponseWriter interface {
	Write(m *dns.Msg) error
}

type DefaultHandler struct {
	// Logger is used for logging. A nil value will disable logging.
	Logger *zap.Logger
	Entry  executable_seq.Executable

	// QueryTimeout limits the timeout value of each query.
	// Default is defaultQueryTimeout.
	QueryTimeout time.Duration

	// RecursionAvailable sets the dns.Msg.RecursionAvailable flag globally.
	RecursionAvailable bool
}

var (
	nopLogger = zap.NewNop()
)

// ServeDNS implements Handler.
// If entry returns an error, a SERVFAIL response will be sent back to client.
// If concurrentLimit is reached, the query will block and wait available token until ctx is done.
func (h *DefaultHandler) ServeDNS(ctx context.Context, req *dns.Msg, w ResponseWriter, meta *query_context.RequestMeta) error {
	// apply timeout to ctx
	ddl := time.Now().Add(h.queryTimeout())
	ctxDdl, ok := ctx.Deadline()
	if !(ok && ctxDdl.Before(ddl)) {
		newCtx, cancel := context.WithDeadline(ctx, ddl)
		defer cancel()
		ctx = newCtx
	}

	qCtx := query_context.NewContext(req, meta)
	err := h.Entry.Exec(ctx, qCtx, nil)
	if err != nil {
		h.logger().Warn("entry returned an err", qCtx.InfoField(), zap.Error(err))
	} else {
		h.logger().Debug("entry returned", qCtx.InfoField(), zap.Stringer("status", qCtx.Status()))
	}

	var respMsg *dns.Msg
	if err != nil || qCtx.Status() == query_context.ContextStatusServerFailed {
		respMsg = new(dns.Msg)
		respMsg.SetReply(req)
		respMsg.Rcode = dns.RcodeServerFailure
	} else {
		respMsg = qCtx.R()
	}

	if respMsg != nil {
		if h.RecursionAvailable {
			respMsg.RecursionAvailable = true
		}

		if err := w.Write(respMsg); err != nil {
			h.logger().Warn("failed to write response", qCtx.InfoField(), zap.Error(err))
		}
	}
	return nil
}

func (h *DefaultHandler) queryTimeout() time.Duration {
	if t := h.QueryTimeout; t > 0 {
		return t
	}
	return defaultQueryTimeout
}

func (h *DefaultHandler) logger() *zap.Logger {
	if l := h.Logger; l != nil {
		return l
	}
	return nopLogger
}

type DummyServerHandler struct {
	T       *testing.T
	WantMsg *dns.Msg
	WantErr error
}

func (d *DummyServerHandler) ServeDNS(_ context.Context, req *dns.Msg, w ResponseWriter, meta *query_context.RequestMeta) error {
	var resp *dns.Msg
	if d.WantMsg != nil {
		resp = d.WantMsg.Copy()
		resp.Id = req.Id
	} else {
		resp = new(dns.Msg)
		resp.SetReply(req)
	}

	if err := w.Write(resp); err != nil {
		d.T.Error(err)
		return nil
	}
	return nil
}
