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
)

type ReuseConnTransport struct {
	ReuseConnOpts

	m          sync.Mutex // protect following fields
	closed     bool
	idledConns []*dnsConn
	conns      map[*dnsConn]struct{}
}

type ReuseConnOpts struct {
	IOOpts
}

func NewReuseConnTransport(opt ReuseConnOpts) *ReuseConnTransport {
	return &ReuseConnTransport{
		ReuseConnOpts: opt,
		conns:         make(map[*dnsConn]struct{}),
	}
}

func (t *ReuseConnTransport) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	const maxAttempt = 3

	attempt := 0
	for {
		attempt++
		conn, isNewConn, err := t.getReusableConn()
		if err != nil {
			return nil, err
		}

		r, err := conn.exchange(ctx, m)
		t.releaseReusableConn(conn, err)
		if err != nil {
			if !isNewConn && attempt <= maxAttempt && ctx.Err() == nil {
				continue
			}
			return nil, err
		}

		return r, nil
	}
}

// getReusableConn returns a *dnsConn.
// Returned bool indicates whether this dnsConn is a reused connection.
// The caller must call releaseReusableConn to release the dnsConn.
func (t *ReuseConnTransport) getReusableConn() (*dnsConn, bool, error) {
	t.m.Lock()
	if t.closed {
		return nil, false, errClosedTransport
	}
	for {
		// prefer to use the latest connection
		dc, _ := slicePopLatest(&t.idledConns)
		if dc == nil { // no idled connection
			break
		}
		if !dc.isClosed() {
			t.m.Unlock()
			return dc, true, nil
		}
		// connection was closed, delete it from the pool and retry
		delete(t.conns, dc)
	}

	// No idled connection, need to dial a one for this query.
	dc := newDnsConn(t.IOOpts)
	t.conns[dc] = struct{}{}
	t.m.Unlock()
	return dc, false, nil
}

// Close closes ReuseConnTransport and all its connections.
// It always returns a nil error.
func (t *ReuseConnTransport) Close() error {
	t.m.Lock()
	defer t.m.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	for conn := range t.conns {
		conn.closeWithErr(errClosedTransport)
	}
	return nil
}

// If err != nil, the released dnsConn will be closed instead of
// returning to the conn pool.
func (t *ReuseConnTransport) releaseReusableConn(c *dnsConn, err error) {
	var closeConn bool
	t.m.Lock()
	// t was closed, this connection is now an orphan and should be closed.
	// In fact, this connection has been closed when t was closed.
	if t.closed {
		closeConn = true
		err = errClosedTransport
	} else {
		// close this connection if it had an error.
		if err != nil {
			delete(t.conns, c)
			closeConn = true
		} else { // looks good, put it into pool.
			sliceAdd(&t.idledConns, c)
		}
	}
	t.m.Unlock()

	if closeConn {
		c.closeWithErr(err)
	}
}
