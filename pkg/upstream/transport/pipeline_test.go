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

package transport

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

type dummyEchoDnsConnOpt struct {
	exchangeErr error
	mcq         int

	closed atomic.Bool

	wantConcurrentExchangeCall int
	waitingExchangeCall        atomic.Int32
	unblockOnce                sync.Once
	unblockExchange            chan struct{}
}

type dummyEchoDnsConn struct {
	opt *dummyEchoDnsConnOpt

	m        sync.Mutex
	reserved int
}

func (dc *dummyEchoDnsConn) ExchangeReserved(ctx context.Context, q []byte) (*[]byte, error) {
	defer dc.WithdrawReserved()

	if dc.opt.waitingExchangeCall.Add(1) == int32(dc.opt.wantConcurrentExchangeCall) {
		dc.opt.unblockOnce.Do(func() { close(dc.opt.unblockExchange) })
	}
	defer dc.opt.waitingExchangeCall.Add(-1)

	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case <-dc.opt.unblockExchange:
		if dc.opt.exchangeErr != nil {
			return nil, dc.opt.exchangeErr
		}
		return copyMsg(q), nil
	}
}

func (dc *dummyEchoDnsConn) WithdrawReserved() {
	dc.m.Lock()
	defer dc.m.Unlock()
	dc.reserved--
	if dc.reserved < 0 {
		panic("negative reserved counter")
	}
}

func (dc *dummyEchoDnsConn) ReserveNewQuery() (_ ReservedExchanger, closed bool) {
	if dc.opt.closed.Load() {
		return nil, true
	}
	dc.m.Lock()
	defer dc.m.Unlock()
	if dc.reserved >= dc.opt.mcq {
		return nil, false
	}
	dc.reserved++
	return dc, false
}

func (dc *dummyEchoDnsConn) Close() error {
	return nil
}

func Test_PipelineTransport(t *testing.T) {
	const (
		mcq                           = 100
		wantConn                      = 10
		wantMaxConcurrentExchangeCall = mcq * wantConn
	)

	r := require.New(t)
	dcControl := &dummyEchoDnsConnOpt{
		mcq:                        mcq,
		unblockExchange:            make(chan struct{}),
		wantConcurrentExchangeCall: wantMaxConcurrentExchangeCall,
	}
	po := PipelineOpts{
		DialContext:                    func(ctx context.Context) (DnsConn, error) { return &dummyEchoDnsConn{opt: dcControl}, nil },
		MaxConcurrentQueryWhileDialing: mcq,
	}
	pt := NewPipelineTransport(po)
	defer pt.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	q := new(dns.Msg)
	q.SetQuestion("test.", dns.TypeA)
	queryPayload, err := q.Pack()
	r.NoError(err)
	wg := new(sync.WaitGroup)
	for i := 0; i < wantMaxConcurrentExchangeCall; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := pt.ExchangeContext(ctx, queryPayload)
			if err != nil {
				t.Error(err)
			}
		}()
		if t.Failed() {
			break
		}
	}
	wg.Wait()

	pt.m.Lock()
	pl := len(pt.conns)
	pt.m.Unlock()

	r.Equal(wantConn, pl)

	dcControl.closed.Store(true)
	_, _ = pt.ExchangeContext(ctx, queryPayload) // remove all closed conn

	pt.m.Lock()
	pl = len(pt.conns)
	pt.m.Unlock()
	r.Equal(1, pl, "all connection should be remove then one will be opened")
}
