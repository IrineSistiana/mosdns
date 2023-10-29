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
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/concurrent_limiter"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/miekg/dns"
	"sync"
	"time"
)

const PluginType = "client_limiter"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	MaxQPS int `yaml:"max_qps"`
	V4Mask int `yaml:"v4_mask"` // default is 32
	V6Mask int `yaml:"v6_mask"` // default is 48
}

var _ coremain.ExecutablePlugin = (*Limiter)(nil)

type Limiter struct {
	*coremain.BP

	closeOnce   sync.Once
	closeNotify chan struct{}
	hpLimiter   *concurrent_limiter.HPClientLimiter
}

func NewLimiter(bp *coremain.BP, args *Args) (*Limiter, error) {
	hpl, err := concurrent_limiter.NewHPClientLimiter(concurrent_limiter.HPLimiterOpts{
		Threshold: args.MaxQPS,
		Interval:  time.Second,
	})
	if err != nil {
		return nil, err
	}
	l := &Limiter{
		BP:          bp,
		hpLimiter:   hpl,
		closeNotify: make(chan struct{}),
	}
	go l.cleanerLoop()
	return l, nil
}

func (l *Limiter) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	addr := qCtx.ReqMeta().ClientAddr
	if !addr.IsValid() {
		return executable_seq.ExecChainNode(ctx, qCtx, next)
	}
	if ok := l.hpLimiter.AcquireToken(addr); !ok {
		r := new(dns.Msg)
		r.SetRcode(qCtx.Q(), dns.RcodeRefused)
		qCtx.SetResponse(r)
		return nil
	}
	return executable_seq.ExecChainNode(ctx, qCtx, next)
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
	return NewLimiter(bp, args.(*Args))
}
