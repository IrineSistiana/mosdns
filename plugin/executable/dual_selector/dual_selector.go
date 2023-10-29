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

package dual_selector

import (
	"context"
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/dnsutils"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/pool"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"time"
)

const PluginType = "dual_selector"

const (
	modePreferIPv4 = iota
	modePreferIPv6

	defaultWaitTimeout      = time.Millisecond * 250
	defaultSubRoutineTimout = time.Second * 5
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
	coremain.RegNewPersetPluginFunc("_prefer_ipv4", func(bp *coremain.BP) (coremain.Plugin, error) {
		return &Selector{BP: bp, mode: modePreferIPv4}, nil
	})
	coremain.RegNewPersetPluginFunc("_prefer_ipv6", func(bp *coremain.BP) (coremain.Plugin, error) {
		return &Selector{BP: bp, mode: modePreferIPv6}, nil
	})
}

type Args struct {
	Mode        int `yaml:"mode"`
	WaitTimeout int `yaml:"wait_timeout"`
}

var _ coremain.ExecutablePlugin = (*Selector)(nil)

type Selector struct {
	*coremain.BP
	mode        int
	waitTimeout time.Duration
}

func (s *Selector) getWaitTimeout() time.Duration {
	if s.waitTimeout <= 0 {
		return defaultWaitTimeout
	}
	return s.waitTimeout
}

// Exec implements handler.Executable.
func (s *Selector) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	q := qCtx.Q()
	if len(q.Question) != 1 { // skip wired query with multiple questions.
		return executable_seq.ExecChainNode(ctx, qCtx, next)
	}

	qtype := q.Question[0].Qtype
	// skip queries that have preferred type or have other unrelated qtypes.
	if (qtype == dns.TypeA && s.mode == modePreferIPv4) || (qtype == dns.TypeAAAA && s.mode == modePreferIPv6) || (qtype != dns.TypeA && qtype != dns.TypeAAAA) {
		return executable_seq.ExecChainNode(ctx, qCtx, next)
	}

	// start reference goroutine
	qCtxRef := qCtx.Copy()
	var refQtype uint16
	if qtype == dns.TypeA {
		refQtype = dns.TypeAAAA
	} else {
		refQtype = dns.TypeA
	}
	qCtxRef.Q().Question[0].Qtype = refQtype

	ddl, ok := ctx.Deadline()
	if !ok {
		ddl = time.Now().Add(defaultSubRoutineTimout)
	}

	shouldBlock := make(chan struct{}, 0)
	shouldPass := make(chan struct{}, 0)
	ctxRef, cancelRef := context.WithDeadline(context.Background(), ddl)
	go func() {
		defer cancelRef()
		err := executable_seq.ExecChainNode(ctxRef, qCtxRef, next)
		if err != nil {
			s.L().Warn("reference query routine err", qCtxRef.InfoField(), zap.Error(err))
			close(shouldPass)
			return
		}
		if r := qCtxRef.R(); r != nil && msgAnsHasRR(r, refQtype) {
			// Target domain has reference type.
			close(shouldBlock)
			return
		}
		close(shouldPass)
		return
	}()

	// start original query goroutine
	doneChan := make(chan error, 1)
	qCtxSub := qCtx.Copy()
	ctxSub, cancelSub := context.WithDeadline(context.Background(), ddl)
	defer cancelSub()
	go func() {
		doneChan <- executable_seq.ExecChainNode(ctxSub, qCtxSub, next)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-shouldBlock: // Reference indicates we should block this query before the original query finished.
		r := dnsutils.GenEmptyReply(q, dns.RcodeSuccess)
		qCtx.SetResponse(r)
		return nil
	case err := <-doneChan: // The original query finished. Waiting for reference.
		waitTimeoutTimer := pool.GetTimer(s.getWaitTimeout())
		defer pool.ReleaseTimer(waitTimeoutTimer)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-shouldBlock:
			r := dnsutils.GenEmptyReply(q, dns.RcodeSuccess)
			qCtx.SetResponse(r)
			return nil
		case <-shouldPass:
			*qCtx = *qCtxSub
			return err
		case <-waitTimeoutTimer.C:
			// We have been waiting the reference query for too long.
			// Something may go wrong. We accept the original reply.
			*qCtx = *qCtxSub
			return err
		}
	}
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return NewDualSelector(bp, args.(*Args)), nil
}

func NewDualSelector(bp *coremain.BP, args *Args) *Selector {
	return &Selector{
		BP:          bp,
		mode:        args.Mode,
		waitTimeout: time.Duration(args.WaitTimeout) * time.Millisecond,
	}
}

func msgAnsHasRR(m *dns.Msg, t uint16) bool {
	if len(m.Answer) == 0 {
		return false
	}

	for _, rr := range m.Answer {
		if rr.Header().Rrtype == t {
			return true
		}
	}
	return false
}
