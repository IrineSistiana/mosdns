//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package handler

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type ServerHandler interface {
	// ServeDNS uses ctx to control deadline, exchanges qCtx, and writes response to w.
	ServeDNS(ctx context.Context, qCtx *Context, w ResponseWriter)
}

// ResponseWriter can write msg to the client.
type ResponseWriter interface {
	Write(m *dns.Msg) (n int, err error)
}

type DefaultServerHandler struct {
	config *DefaultServerHandlerConfig

	limiter *concurrentLimiter // if it's nil, means no limit.
}

type DefaultServerHandlerConfig struct {
	// Logger is used for logging, it cannot be nil.
	Logger *zap.Logger
	// Entry is the entry ExecutablePlugin's tag. This shouldn't be empty.
	Entry string
	// ConcurrentLimit controls the max concurrent queries.
	// If ConcurrentLimit <= 0, means no limit.
	ConcurrentLimit int
}

// NewDefaultServerHandler:
// concurrentLimit <= 0 means no concurrent limit.
// Also see DefaultServerHandler.ServeDNS.
func NewDefaultServerHandler(config *DefaultServerHandlerConfig) *DefaultServerHandler {
	h := &DefaultServerHandler{config: config}

	if config.ConcurrentLimit > 0 {
		h.limiter = newConcurrentLimiter(config.ConcurrentLimit)
	}
	return h
}

// ServeDNS:
// If entry returns an err, a SERVFAIL response will be sent back to client.
// If concurrentLimit is reached, the query will block and wait available token until ctx is done.
func (h *DefaultServerHandler) ServeDNS(ctx context.Context, qCtx *Context, w ResponseWriter) {
	if h.limiter != nil {
		select {
		case <-h.limiter.acquire():
			defer h.limiter.release()
		case <-ctx.Done():
			// silently drop this query
			return
		}
	}

	h.config.Logger.Debug("exec entry", qCtx.InfoField(), zap.String("entry", h.config.Entry))
	err := h.execEntry(ctx, qCtx)

	if err != nil {
		h.config.Logger.Warn("entry returned an err", qCtx.InfoField(), zap.Error(err))
	} else {
		h.config.Logger.Debug("entry returned", qCtx.InfoField(), zap.Stringer("status", qCtx.Status()))
	}

	var r *dns.Msg
	if err != nil || qCtx.Status() == ContextStatusServerFailed {
		r = new(dns.Msg)
		r.SetReply(qCtx.Q())
		r.Rcode = dns.RcodeServerFailure
	} else {
		r = qCtx.R()
	}

	if r != nil {
		if _, err := w.Write(r); err != nil {
			h.config.Logger.Warn("write response", qCtx.InfoField(), zap.Error(err))
		}
	}
}

func (h *DefaultServerHandler) execEntry(ctx context.Context, qCtx *Context) error {
	p, err := GetPlugin(h.config.Entry)
	if err != nil {
		return err
	}

	err = p.Exec(ctx, qCtx)
	if err != nil {
		return err
	}

	err = qCtx.ExecDefer(ctx, qCtx)
	if err != nil {
		return err
	}

	return nil
}

// concurrentLimiter
type concurrentLimiter struct {
	bucket chan struct{}
}

// newConcurrentLimiter returns a concurrentLimiter, max must > 0.
func newConcurrentLimiter(max int) *concurrentLimiter {
	if max <= 0 {
		panic(fmt.Sprintf("concurrentLimiter: invalid max arg: %d", max))
	}

	bucket := make(chan struct{}, max)
For:
	for {
		select {
		case bucket <- struct{}{}:
		default:
			break For
		}
	}

	return &concurrentLimiter{bucket: bucket}
}

func (l *concurrentLimiter) acquire() <-chan struct{} {
	return l.bucket
}

func (l *concurrentLimiter) release() {
	select {
	case l.bucket <- struct{}{}:
	default:
		panic("concurrentLimiter: bucket overflow")
	}
}

func (l *concurrentLimiter) available() int {
	return len(l.bucket)
}
