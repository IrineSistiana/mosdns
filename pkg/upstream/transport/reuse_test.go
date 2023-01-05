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
	"github.com/miekg/dns"
	"sync"
	"testing"
	"time"
)

// Leak and race tests.
func Test_ReuseConnTransport(t *testing.T) {
	// no error
	testReuseConnTransport(t, IOOpts{
		DialFunc:  dial,
		WriteFunc: write,
		ReadFunc:  read,
	})

	// dnsConn error
	testReuseConnTransport(t, IOOpts{
		DialFunc:  dialErr,
		WriteFunc: write,
		ReadFunc:  read,
	})
	testReuseConnTransport(t, IOOpts{
		DialFunc:  dial,
		WriteFunc: writeErr,
		ReadFunc:  read,
	})
	testReuseConnTransport(t, IOOpts{
		DialFunc:  dial,
		WriteFunc: write,
		ReadFunc:  readErr,
	})

	// random err
	testReuseConnTransport(t, IOOpts{
		DialFunc:  dialErrP,
		WriteFunc: writeErrP,
		ReadFunc:  readErrP,
	})
}

func testReuseConnTransport(t *testing.T, ioOpts IOOpts) {
	t.Helper()
	po := ReuseConnOpts{
		IOOpts: ioOpts,
	}
	rt := NewReuseConnTransport(po)
	defer rt.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	q := new(dns.Msg)
	q.SetQuestion("test.", dns.TypeA)
	connsNum := 4
	for l := 0; l < 4; l++ {
		wg := new(sync.WaitGroup)
		for i := 0; i < connsNum; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = rt.ExchangeContext(ctx, q)
			}()
		}
		wg.Wait()
	}

	rt.m.Lock()
	pl := len(rt.conns)
	il := len(rt.idledConns)
	rt.m.Unlock()
	if pl > connsNum {
		t.Fatalf("max %d active conn, but got %d active conn(s)", connsNum, pl)
	}
	if il > connsNum {
		t.Fatalf("max %d active conn, but got %d idled conn(s)", connsNum, pl)
	}
}
