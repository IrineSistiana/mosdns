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

package client_limiter

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/concurrent_limiter"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"net/netip"
	"sync"
	"time"
)

const PluginType = "client_limiter"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	MaxQPS int `yaml:"max_qps"`
}

var _ coremain.ExecutablePlugin = (*Limiter)(nil)

type Limiter struct {
	*coremain.BP

	closeOnce   sync.Once
	closeNotify chan struct{}

	v4Mask, v6Mask int
	hpLimiter      *concurrent_limiter.HPClientLimiter
}

func NewLimiter(bp *coremain.BP, args *Args) *Limiter {
	l := &Limiter{
		BP:          bp,
		hpLimiter:   concurrent_limiter.NewHPClientLimiter(args.MaxQPS),
		closeNotify: make(chan struct{}),
	}
	go l.cleanerLoop()
	return l
}

func (l *Limiter) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	addr, ok := l.parseClientAddr(qCtx)
	if !ok {
		return executable_seq.ExecChainNode(ctx, qCtx, next)
	}
	if ok := l.hpLimiter.Acquire(addr); !ok {
		r := new(dns.Msg)
		r.SetRcode(qCtx.Q(), dns.RcodeRefused)
		qCtx.SetResponse(r, query_context.ContextStatusRejected)
		return nil
	}
	return executable_seq.ExecChainNode(ctx, qCtx, next)
}

func (l *Limiter) parseClientAddr(qCtx *query_context.Context) (netip.Addr, bool) {
	meta := qCtx.ReqMeta()
	if meta == nil {
		return netip.Addr{}, false
	}
	ip := meta.ClientIP
	if ip == nil {
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		l.BP.L().Error("query context has a invalid client ip", zap.Binary("ip", ip))
		return netip.Addr{}, false
	}
	return addr, true
}

func (l *Limiter) Close() error {
	l.closeOnce.Do(func() {
		close(l.closeNotify)
	})
	return nil
}

func (l *Limiter) cleanerLoop() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			l.hpLimiter.GC(now)
		case <-l.closeNotify:
			return
		}
	}
}

// Init is a handler.NewPluginFunc.
func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return NewLimiter(bp, args.(*Args)), nil
}
