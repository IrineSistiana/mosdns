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

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"go.uber.org/zap"
)

// ReuseConnTransport is for old tcp protocol. (no pipelining)
type ReuseConnTransport struct {
	ReuseConnOpts

	logger    *zap.Logger // non-nil
	ctx       context.Context
	ctxCancel context.CancelCauseFunc

	m         sync.Mutex // protect following fields
	closed    bool
	idleConns map[*reusableConn]struct{}
	conns     map[*reusableConn]struct{}
}

type ReuseConnOpts struct {
	// DialContext specifies the method to dial a connection to the server.
	// DialContext MUST NOT be nil.
	DialContext func(ctx context.Context) (NetConn, error)

	// DialTimeout specifies the timeout for DialFunc.
	// Default is defaultDialTimeout.
	DialTimeout time.Duration

	// Default is defaultIdleTimeout
	IdleTimeout time.Duration

	Logger *zap.Logger
}

func NewReuseConnTransport(opt ReuseConnOpts) *ReuseConnTransport {
	ctx, cancel := context.WithCancelCause(context.Background())
	t := &ReuseConnTransport{
		ctx:           ctx,
		ctxCancel:     cancel,
		ReuseConnOpts: opt,
		idleConns:     make(map[*reusableConn]struct{}),
		conns:         make(map[*reusableConn]struct{}),
	}
	if opt.Logger != nil {
		t.logger = opt.Logger
	} else {
		t.logger = nopLogger
	}
	return t
}

func (t *ReuseConnTransport) ExchangeContext(ctx context.Context, m []byte) (*[]byte, error) {
	const maxRetry = 2

	retry := 0
	for {
		var isNewConn bool
		c, err := t.getIdleConn()
		if err != nil {
			return nil, err
		}
		if c == nil {
			isNewConn = true
			c, err = t.wantNewConn(ctx)
			if err != nil {
				return nil, err
			}
		}

		queryPayload, err := copyMsgWithLenHdr(m)
		if err != nil {
			return nil, err
		}

		type res struct {
			resp *[]byte
			err  error
		}
		resChan := make(chan res, 1)
		go func() {
			defer pool.ReleaseBuf(queryPayload)
			resp, err := c.exchange(queryPayload)
			t.releaseReusableConn(c, err != nil)
			resChan <- res{resp: resp, err: err}
		}()

		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case res := <-resChan:
			r, err := res.resp, res.err
			if err != nil {
				if !isNewConn && retry <= maxRetry {
					retry++
					continue // retry if c is a reused connection.
				}
				return nil, err
			}
			return r, nil
		}

	}
}

func (t *ReuseConnTransport) wantNewConn(ctx context.Context) (*reusableConn, error) {
	type dialRes struct {
		c   *reusableConn
		err error
	}

	dialChan := make(chan dialRes)
	go func() {
		dialTimeout := t.DialTimeout
		if dialTimeout <= 0 {
			dialTimeout = defaultDialTimeout
		}
		idleTimeout := t.IdleTimeout
		if idleTimeout <= 0 {
			idleTimeout = defaultIdleTimeout
		}

		ctx, cancel := context.WithTimeout(t.ctx, dialTimeout)
		defer cancel()

		var rc *reusableConn
		c, err := t.DialContext(ctx)
		if err != nil {
			t.logger.Check(zap.WarnLevel, "fail to dial reusable conn").Write(zap.Error(err))
		}
		if c != nil {
			rc = newReusableConn(c, idleTimeout)
			t.trackNewReusableConn(rc)
		}

		select {
		case dialChan <- dialRes{c: rc, err: err}:
		case <-ctx.Done():
			if rc != nil {
				t.releaseReusableConn(rc, false)
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case res := <-dialChan:
		return res.c, res.err
	}
}

// getIdleConn returns a *reusableConn from conn pool, or nil if no conn
// is idle.
// The caller must call releaseReusableConn to release the reusableConn.
func (t *ReuseConnTransport) getIdleConn() (*reusableConn, error) {
	t.m.Lock()
	defer t.m.Unlock()
	if t.closed {
		return nil, ErrClosedTransport
	}

	for c := range t.idleConns {
		delete(t.idleConns, c)
		if ok := c.stopIdleTimer(); !ok { // idle timer was fired and connection was closed
			delete(t.conns, c)
			continue
		}
		return c, nil
	}
	return nil, nil
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
	for c := range t.conns {
		delete(t.conns, c)
		delete(t.idleConns, c)
		c.Close()
	}
	t.ctxCancel(ErrClosedTransport)
	return nil
}

// returning c to the conn pool.
func (t *ReuseConnTransport) releaseReusableConn(c *reusableConn, dead bool) {
	t.m.Lock()
	// t was closed, this connection has been closed when t was closed.
	if t.closed {
		t.m.Unlock()
		return
	} else {
		// close this connection if it had an error.
		if dead {
			delete(t.conns, c)
		} else { // looks good, put it into pool.
			c.startIdleTimer()
			t.idleConns[c] = struct{}{}
		}
	}
	t.m.Unlock()

	if dead {
		c.Close()
	}
}

func (t *ReuseConnTransport) trackNewReusableConn(c *reusableConn) {
	t.m.Lock()
	if t.closed { // t was closed.
		c.Close()
		t.m.Unlock()
		return
	}
	t.conns[c] = struct{}{}
	t.m.Unlock()
}

type reusableConn struct {
	c           NetConn
	idleTimeout time.Duration
	idleTimer   *time.Timer
}

// idleTimeout must be valid.
func newReusableConn(c NetConn, idleTimeout time.Duration) *reusableConn {
	rc := &reusableConn{
		c:           c,
		idleTimeout: idleTimeout,
	}
	rc.idleTimer = time.AfterFunc(idleTimeout, func() {
		c.Close()
	})
	return rc
}

func (c *reusableConn) stopIdleTimer() bool {
	return c.idleTimer.Stop()
}

func (c *reusableConn) startIdleTimer() {
	c.idleTimer.Reset(c.idleTimeout)
}

func (c *reusableConn) Close() error {
	c.idleTimer.Stop()
	return c.c.Close()
}

func (c *reusableConn) exchange(q *[]byte) (*[]byte, error) {
	c.c.SetDeadline(time.Now().Add(reuseConnQueryTimeout))
	_, err := c.c.Write(*q)
	if err != nil {
		return nil, err
	}
	return dnsutils.ReadRawMsgFromTCP(c.c)
}
