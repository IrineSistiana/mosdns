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
	"testing"
	"time"

	"github.com/miekg/dns"
)

// Leak and race tests.
func Test_PipelineTransport(t *testing.T) {
	// no error
	testPipelineTransport(t, IOOpts{
		DialFunc:  dial,
		WriteFunc: write,
		ReadFunc:  read,
	})

	// dnsConn error
	testPipelineTransport(t, IOOpts{
		DialFunc:  dialErr,
		WriteFunc: write,
		ReadFunc:  read,
	})
	testPipelineTransport(t, IOOpts{
		DialFunc:  dial,
		WriteFunc: writeErr,
		ReadFunc:  read,
	})
	testPipelineTransport(t, IOOpts{
		DialFunc:  dial,
		WriteFunc: write,
		ReadFunc:  readErr,
	})

	// random err
	testPipelineTransport(t, IOOpts{
		DialFunc:  dialErrP,
		WriteFunc: writeErrP,
		ReadFunc:  readErrP,
	})
}

func testPipelineTransport(t *testing.T, ioOpts IOOpts) {
	t.Helper()
	po := PipelineOpts{
		IOOpts:  ioOpts,
		MaxConn: 4,
	}
	pt := NewPipelineTransport(po)
	defer pt.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	q := new(dns.Msg)
	q.SetQuestion("test.", dns.TypeA)
	wg := new(sync.WaitGroup)
	for i := 0; i < po.MaxConn*pipelineBusyQueueLen*2; i++ { // *2 ensures a new connection will be opened.
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pt.ExchangeContext(ctx, q)
		}()
	}
	wg.Wait()

	pt.m.Lock()
	pl := len(pt.activeConns)
	pt.m.Unlock()
	if pl > po.MaxConn {
		t.Fatalf("max %d active conn, but got %d active conn(s)", po.MaxConn, pl)
	}
}
