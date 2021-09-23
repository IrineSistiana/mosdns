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

package dns_handler

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/concurrent_limiter"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"sync"
	"testing"
	"time"
)

const (
	defaultLazyUpdateTimeout = time.Second * 5
)

type Handler interface {
	// ServeDNS uses ctx to control deadline, exchanges qCtx, and writes response to w.
	ServeDNS(ctx context.Context, qCtx *handler.Context, w ResponseWriter)
}

// ResponseWriter can write msg to the client.
type ResponseWriter interface {
	Write(m *dns.Msg) (n int, err error)
}

type DefaultHandler struct {
	// Logger is used for logging. A nil value will disable logging.
	Logger *zap.Logger

	// Entry is the entry ExecutablePlugin's tag. This cannot be nil.
	Entry executable_seq.ExecutableNode

	// ConcurrentLimit controls the max concurrent queries for the DefaultHandler.
	// If ConcurrentLimit <= 0, means no limit.
	// When calling DefaultHandler.ServeDNS(), if a query exceeds the limit, it will wait on a FIFO queue until
	// - its ctx is done or currently there are more than 3 x ConcurrentLimit queries waiting -> The query will be dropped silently.
	// - it can be proceeded -> Normal procedure.
	ConcurrentLimit int

	// If Cache is not nil, cache will be enabled.
	Cache        cache.Backend
	LazyCacheTTL int // If LazyCacheTTL > 0. Lazy cache mode will be enabled.

	initOnce sync.Once // init the followings
	logger   *zap.Logger
	limiter  *concurrent_limiter.ConcurrentLimiter // if it's nil, means no limit.

	lazyUpdateSF singleflight.Group
}

// ServeDNS
// If entry returns an error, a SERVFAIL response will be sent back to client.
// If concurrentLimit is reached, the query will block and wait available token until ctx is done.
func (h *DefaultHandler) ServeDNS(ctx context.Context, qCtx *handler.Context, w ResponseWriter) {
	h.initOnce.Do(func() {
		if h.Logger != nil {
			h.logger = h.Logger
		} else {
			h.logger = zap.NewNop()
		}
		if h.ConcurrentLimit > 0 {
			h.limiter = concurrent_limiter.NewConcurrentLimiter(h.ConcurrentLimit)
		}
	})

	if h.limiter != nil {
		if int(h.limiter.Running()) > h.limiter.Max() { // too many waiting query, silently drop it.
			return
		}

		select {
		case h.limiter.Wait() <- struct{}{}:
			defer h.limiter.Done()
		case <-ctx.Done():
			return // ctx timeout, silently drop it.
		}
	}

	err := h.execEntry(ctx, qCtx)

	if err != nil {
		h.logger.Warn("entry returned an err", qCtx.InfoField(), zap.Error(err))
	} else {
		h.logger.Debug("entry returned", qCtx.InfoField(), zap.Stringer("status", qCtx.Status()))
	}

	var r *dns.Msg
	if err != nil || qCtx.Status() == handler.ContextStatusServerFailed {
		r = new(dns.Msg)
		r.SetReply(qCtx.Q())
		r.Rcode = dns.RcodeServerFailure
	} else {
		r = qCtx.R()
	}

	if r != nil {
		if _, err := w.Write(r); err != nil {
			h.logger.Warn("write response", qCtx.InfoField(), zap.Error(err))
		}
	}
}

func (h *DefaultHandler) execEntry(ctx context.Context, qCtx *handler.Context) error {
	if h.Cache == nil {
		_, err := executable_seq.ExecRoot(ctx, qCtx, h.logger, h.Entry)
		return err
	}

	q := qCtx.Q()
	key, err := utils.GetMsgKey(q, 0)
	if err != nil {
		return fmt.Errorf("unable to get msg key, %w", err)
	}

	// lookup in cache
	r, storedTime, _, err := h.Cache.Get(ctx, key, h.LazyCacheTTL > 0)
	if err != nil {
		return fmt.Errorf("unable to access cache, %w", err)
	}

	// cache hit
	if r != nil {
		// change msg id to query
		r.Id = q.Id

		if h.LazyCacheTTL > 0 && storedTime.Add(time.Duration(dnsutils.GetMinimalTTL(r))*time.Second).Before(time.Now()) { // lazy update enabled and expired cache hit
			// prepare a response with 1 ttl
			dnsutils.SetTTL(r, 1)
			h.logger.Debug("expired cache hit", qCtx.InfoField())
			qCtx.SetResponse(r, handler.ContextStatusResponded)

			// start a goroutine to update cache
			lazyUpdateDdl, ok := ctx.Deadline()
			if !ok {
				lazyUpdateDdl = time.Now().Add(defaultLazyUpdateTimeout)
			}
			lazyQCtx := qCtx.CopyNoR()
			lazyUpdateFunc := func() (interface{}, error) {
				h.logger.Debug("start lazy cache update", lazyQCtx.InfoField(), zap.Error(err))
				defer h.lazyUpdateSF.Forget(key)
				lazyCtx, cancel := context.WithDeadline(context.Background(), lazyUpdateDdl)
				defer cancel()

				_, err := executable_seq.ExecRoot(lazyCtx, lazyQCtx, h.logger, h.Entry)
				if err != nil {
					h.logger.Warn("failed to update lazy cache", lazyQCtx.InfoField(), zap.Error(err))
				}

				r := lazyQCtx.R()
				if r != nil && cacheAble(r) {
					err := h.storeMsg(ctx, key, r)
					if err != nil {
						h.logger.Warn("failed to store lazy cache", lazyQCtx.InfoField(), zap.Error(err))
					}
				}
				h.logger.Debug("lazy cache updated", lazyQCtx.InfoField(), zap.Error(err))
				return nil, nil
			}
			h.lazyUpdateSF.DoChan(key, lazyUpdateFunc) // DoChan won't block this goroutine

			return nil
		}

		// cache hit but not expired
		dnsutils.SubtractTTL(r, uint32(time.Since(storedTime).Seconds()))
		h.logger.Debug("cache hit", qCtx.InfoField())
		qCtx.SetResponse(r, handler.ContextStatusResponded)
		return nil
	}

	// cache miss, run the entry
	_, err = executable_seq.ExecRoot(ctx, qCtx, h.logger, h.Entry)
	r = qCtx.R()
	if r != nil && cacheAble(r) {
		err := h.storeMsg(ctx, key, r)
		if err != nil {
			h.logger.Warn("failed to store lazy cache", qCtx.InfoField(), zap.Error(err))
		}
	}
	return err
}

func cacheAble(r *dns.Msg) bool {
	return r.Rcode == dns.RcodeSuccess && r.Truncated == false && len(r.Answer) != 0
}

func (h *DefaultHandler) storeMsg(ctx context.Context, key string, r *dns.Msg) error {
	now := time.Now()
	var expirationTime time.Time
	if h.LazyCacheTTL > 0 {
		expirationTime = now.Add(time.Duration(h.LazyCacheTTL) * time.Second)
	} else {
		expirationTime = now.Add(time.Duration(dnsutils.GetMinimalTTL(r)) * time.Second)
	}
	return h.Cache.Store(ctx, key, r, now, expirationTime)
}

type DummyServerHandler struct {
	T       *testing.T
	WantMsg *dns.Msg
	WantErr error
}

func (d *DummyServerHandler) ServeDNS(_ context.Context, qCtx *handler.Context, w ResponseWriter) {
	var r *dns.Msg
	if d.WantMsg != nil {
		r = d.WantMsg.Copy()
		r.Id = qCtx.Q().Id
	} else {
		r = new(dns.Msg)
		r.SetReply(qCtx.Q())
	}

	_, err := w.Write(r)
	if err != nil {
		d.T.Errorf("DummyServerHandler: %v", err)
	}
}
