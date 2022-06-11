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

package single_flight

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type dummyNext struct {
	c uint32
}

func (d *dummyNext) Exec(ctx context.Context, qCtx *query_context.Context, _ executable_seq.ExecutableChainNode) error {
	atomic.AddUint32(&d.c, 1)
	<-ctx.Done()
	r := new(dns.Msg)
	r.SetReply(qCtx.Q())
	qCtx.SetResponse(r, query_context.ContextStatusResponded)
	return nil
}

func TestSingleFlight_Exec(t *testing.T) {
	sfg := new(SingleFlight)

	var nextNodes []executable_seq.ExecutableChainNode
	for i := 0; i < 5; i++ {
		nextNode := executable_seq.WrapExecutable(new(dummyNext))
		nextNodes = append(nextNodes, nextNode)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := new(sync.WaitGroup)
	for _, next := range nextNodes {
		for i := 0; i < 5; i++ {
			wg.Add(1)
			next := next
			go func() {
				defer wg.Done()
				m := new(dns.Msg)
				m.SetQuestion("example.", dns.TypeA)
				m.Id = dns.Id()
				qCtx := query_context.NewContext(m, nil)
				if err := sfg.Exec(ctx, qCtx, next); err != nil {
					t.Errorf("wanted err: nil, but got: %v", err)
				}
				// check msg id
				if m.Id != qCtx.R().Id {
					t.Error("msg id mismatch")
				}
			}()
		}
	}

	time.Sleep(time.Millisecond * 100)
	cancel()
	wg.Wait()

	for _, next := range nextNodes {
		c := next.(*executable_seq.ExecutableNodeWrapper).Executable.(*dummyNext).c
		if c != 1 {
			t.Fatalf("next was called %d times", c)
		}
	}

}
