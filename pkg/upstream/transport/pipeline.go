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
	"time"

	"go.uber.org/zap"
)

// PipelineTransport will pipeline queries as RFC 7766 6.2.1.1 suggested.
// It also can reuse udp socket. Since dns over udp is some kind of "pipeline".
type PipelineTransport struct {
	m      sync.Mutex // protect following fields
	closed bool
	conns  map[*lazyDnsConn]struct{}

	dialFunc         func(ctx context.Context) (DnsConn, error)
	dialTimeout      time.Duration
	maxLazyConnQueue int
	logger           *zap.Logger // not nil
}

type PipelineOpts struct {
	// DialContext specifies the method to dial a connection to the server.
	// DialContext MUST NOT be nil.
	DialContext func(ctx context.Context) (DnsConn, error)

	// DialTimeout specifies the timeout for DialFunc.
	// Default is defaultDialTimeout.
	DialTimeout time.Duration

	// When connection is dialing, how many queries can be queued up in that
	// connection. Default is defaultLazyConnMaxConcurrentQuery.
	// Note: If the connection turns out having a smaller limit, part of queued up
	// queries will fail.
	MaxConcurrentQueryWhileDialing int

	Logger *zap.Logger
}

func NewPipelineTransport(opt PipelineOpts) *PipelineTransport {
	t := &PipelineTransport{
		conns: make(map[*lazyDnsConn]struct{}),
	}
	t.dialFunc = opt.DialContext
	setDefaultGZ(&t.dialTimeout, opt.DialTimeout, defaultDialTimeout)
	setDefaultGZ(&t.maxLazyConnQueue, opt.MaxConcurrentQueryWhileDialing, defaultMaxLazyConnQueue)
	setNonNilLogger(&t.logger, opt.Logger)

	return t
}

func (t *PipelineTransport) ExchangeContext(ctx context.Context, m []byte) (*[]byte, error) {
	const maxRetry = 2
	retry := 0
	for {
		dc, isNewConn, err := t.getReservedExchanger()
		if err != nil {
			return nil, err
		}
		r, err := dc.ExchangeReserved(ctx, m)
		if err != nil {
			// Reused connection may not stable.
			// Try to re-send this query if it failed on a reused connection.
			if !isNewConn && retry < maxRetry && ctx.Err() == nil {
				retry++
				continue
			}
			return nil, err
		}
		return r, nil
	}
}

// Close closes PipelineTransport and all its connections.
// It always returns a nil error.
func (t *PipelineTransport) Close() error {
	t.m.Lock()
	defer t.m.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	for conn := range t.conns {
		conn.Close()
	}
	return nil
}

func (t *PipelineTransport) getReservedExchanger() (_ ReservedExchanger, isNewConn bool, err error) {
	t.m.Lock()
	if t.closed {
		err = ErrClosedTransport
		t.m.Unlock()
		return
	}

	var rxc ReservedExchanger
	const maxReserveAttempt = 16
	reserveAttempt := 0
	for c := range t.conns {
		var closed bool
		rxc, closed = c.ReserveNewQuery()
		if closed {
			delete(t.conns, c)
		}
		if rxc != nil {
			break
		} else {
			reserveAttempt++
			if reserveAttempt > maxReserveAttempt {
				break
			}
		}
	}

	// Dial a new connection
	if rxc == nil {
		c := newLazyDnsConn(t.dialFunc, t.dialTimeout, t.maxLazyConnQueue, t.logger)
		rxc, _ = c.ReserveNewQuery() // ignore the closed error for new lazy connection
		isNewConn = true
		t.conns[c] = struct{}{}
	}
	t.m.Unlock()

	if rxc == nil {
		isNewConn = false
		err = ErrNewConnCannotReserveQueryExchanger
	}
	return rxc, isNewConn, err
}
