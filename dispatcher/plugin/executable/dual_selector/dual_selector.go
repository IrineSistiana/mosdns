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

package dual_selector

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/pool"
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
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(&Selector{
		BP:   handler.NewBP("_prefer_ipv4", PluginType),
		mode: modePreferIPv4,
	}, true)
	handler.MustRegPlugin(&Selector{
		BP:   handler.NewBP("_prefer_ipv6", PluginType),
		mode: modePreferIPv6,
	}, true)
}

type Args struct {
	Mode        int `yaml:"mode"`
	WaitTimeout int `yaml:"wait_timeout"`
}

var _ handler.ExecutablePlugin = (*Selector)(nil)

type Selector struct {
	*handler.BP
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
func (s *Selector) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	q := qCtx.Q()
	if len(q.Question) != 1 { // skip wired query with multiple questions.
		return handler.ExecChainNode(ctx, qCtx, next)
	}

	qtype := q.Question[0].Qtype
	// skip queries that have preferred type or have other unrelated qtypes.
	if (qtype == dns.TypeA && s.mode == modePreferIPv4) || (qtype == dns.TypeAAAA && s.mode == modePreferIPv6) || (qtype != dns.TypeA && qtype != dns.TypeAAAA) {
		return handler.ExecChainNode(ctx, qCtx, next)
	}

	runSubQuery := func() (chan error, chan error, *handler.Context, *handler.Context) {
		qCtxV4 := qCtx.Copy()
		qCtxV4.Q().Question[0].Qtype = dns.TypeA
		qCtxV6 := qCtx.Copy()
		qCtxV6.Q().Question[0].Qtype = dns.TypeAAAA

		doneChanV4 := make(chan error, 1)
		doneChanV6 := make(chan error, 1)

		ddl, ok := ctx.Deadline()
		if !ok {
			ddl = time.Now().Add(defaultSubRoutineTimout)
		}
		ctxV4, cancelV4 := context.WithDeadline(context.Background(), ddl)
		ctxV6, cancelV6 := context.WithDeadline(context.Background(), ddl)

		go func() {
			doneChanV4 <- handler.ExecChainNode(ctxV4, qCtxV4, next)
			close(doneChanV4)
			cancelV4()
		}()
		go func() {
			doneChanV6 <- handler.ExecChainNode(ctxV6, qCtxV6, next)
			close(doneChanV6)
			cancelV6()
		}()
		return doneChanV4, doneChanV6, qCtxV4, qCtxV6
	}

	switch s.mode {
	case modePreferIPv4: // undergoing query is an AAAA.
		doneChanV4, doneChanV6, qCtxV4, qCtxV6 := runSubQuery()
		return s.waitAndBlock(ctx, doneChanV6, doneChanV4, qCtxV6, qCtxV4, qCtx, dns.TypeA)
	case modePreferIPv6: // undergoing query is an A.
		doneChanV4, doneChanV6, qCtxV4, qCtxV6 := runSubQuery()
		return s.waitAndBlock(ctx, doneChanV4, doneChanV6, qCtxV4, qCtxV6, qCtx, dns.TypeAAAA)
	default:
		return fmt.Errorf("invalid mode: %d", s.mode)
	}
}

// waitAndBlock waits dual replies and blocks the original query if
// the reference returned a valid reply and has the wanted RR type (typRef).
func (s *Selector) waitAndBlock(ctx context.Context, doneChan, doneChanRef chan error, qCtx, qCtxRef, rootQCtx *handler.Context, typRef uint16) error {
	waitTimeoutTimer := pool.GetTimer(s.getWaitTimeout())
	defer pool.ReleaseTimer(waitTimeoutTimer)

	var lazyDoneChan chan error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-doneChanRef: // Reference goroutine done
			if err != nil {
				s.L().Warn("reference query routine err", qCtxRef.InfoField(), zap.Error(err))
			} else {
				if rRef := qCtxRef.R(); rRef != nil && msgAnsHasRR(rRef, typRef) {
					// Target domain has one or more reference records. Block the original query.
					r := dnsutils.GenEmptyReply(rootQCtx.Q(), dns.RcodeSuccess)
					rootQCtx.SetResponse(r, handler.ContextStatusResponded)
					return nil
				}
			}

			// reference sub query failed or target domain does not have an reference record.
			// Waiting for the original reply.
			doneChanRef = nil
			lazyDoneChan = doneChan

		case <-waitTimeoutTimer.C:
			// We have been waiting the reference query for too long.
			// Something may go wrong. We, for now, start to accept the original reply.
			lazyDoneChan = doneChan

		case err := <-lazyDoneChan:
			qCtx.CopyTo(rootQCtx)
			return err
		}
	}
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return NewDualSelector(bp, args.(*Args)), nil
}

func NewDualSelector(bp *handler.BP, args *Args) *Selector {
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
