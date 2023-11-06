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
	"errors"
	"net"
	"sync"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"go.uber.org/zap"
)

const (
	// Most servers will send SERVFAIL after 3~5s. If no resp, connection may be dead.
	reuseConnQueryTimeout = time.Second * 6
)

// ReuseConnTransport is for old tcp protocol. (no pipelining)
type ReuseConnTransport struct {
	dialFunc    func(ctx context.Context) (NetConn, error)
	dialTimeout time.Duration
	idleTimeout time.Duration
	logger      *zap.Logger // non-nil
	ctx         context.Context
	ctxCancel   context.CancelCauseFunc

	m         sync.Mutex // protect following fields
	closed    bool
	idleConns map[*reusableConn]struct{}
	conns     map[*reusableConn]struct{}

	// for testing
	testWaitRespTimeout time.Duration
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
		ctx:       ctx,
		ctxCancel: cancel,
		idleConns: make(map[*reusableConn]struct{}),
		conns:     make(map[*reusableConn]struct{}),
	}
	t.dialFunc = opt.DialContext
	setDefaultGZ(&t.dialTimeout, opt.DialTimeout, defaultDialTimeout)
	setDefaultGZ(&t.idleTimeout, opt.IdleTimeout, defaultIdleTimeout)
	setNonNilLogger(&t.logger, opt.Logger)

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
			c, err = t.getNewConn(ctx)
			if err != nil {
				return nil, err
			}
		}

		queryPayload, err := copyMsgWithLenHdr(m)
		if err != nil {
			return nil, err
		}

		resp, err := c.exchange(ctx, queryPayload)
		if err != nil {
			if !isNewConn && retry <= maxRetry {
				retry++
				continue // retry if c is a reused connection.
			}
			return nil, err
		}
		return resp, nil
	}
}

// getNewConn dial a *reusableConn.
// The caller must call releaseReusableConn to release the reusableConn.
func (t *ReuseConnTransport) getNewConn(ctx context.Context) (*reusableConn, error) {
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type dialRes struct {
		c   *reusableConn
		err error
	}
	dialChan := make(chan dialRes)
	go func() {
		dialCtx, cancelDial := context.WithTimeout(t.ctx, t.dialTimeout)
		defer cancelDial()

		var rc *reusableConn
		c, err := t.dialFunc(dialCtx)
		if err != nil {
			t.logger.Check(zap.WarnLevel, "fail to dial reusable conn").Write(zap.Error(err))
		}
		if c != nil {
			rc = t.newReusableConn(c)
			if rc == nil { // transport closed
				c.Close()
				rc = nil
				err = ErrClosedTransport
			}
		}

		select {
		case dialChan <- dialRes{c: rc, err: err}:
		case <-callCtx.Done(): // caller canceled getNewConn() call
			if rc != nil { // put this conn to pool
				t.setIdle(rc)
			}
		}
	}()

	select {
	case <-callCtx.Done():
		return nil, context.Cause(ctx)
	case <-t.ctx.Done():
		return nil, context.Cause(t.ctx)
	case res := <-dialChan:
		return res.c, res.err
	}
}

func (t *ReuseConnTransport) setIdle(c *reusableConn) {
	t.m.Lock()
	defer t.m.Unlock()
	if t.closed {
		return
	}
	if _, ok := t.conns[c]; ok {
		t.idleConns[c] = struct{}{}
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
		c.closeWithErrByTransport(ErrClosedTransport)
	}
	t.ctxCancel(ErrClosedTransport)
	return nil
}

type reusableConn struct {
	c NetConn
	t *ReuseConnTransport

	m           sync.Mutex
	waitingResp chan *[]byte

	closeOnce   sync.Once
	closeNotify chan struct{}
	closeErr    error
}

// return nil if transport was closed
func (t *ReuseConnTransport) newReusableConn(c NetConn) *reusableConn {
	rc := &reusableConn{
		c:           c,
		t:           t,
		closeNotify: make(chan struct{}),
	}

	t.m.Lock()
	if t.closed { // t was closed.
		t.m.Unlock()
		return nil
	}
	t.conns[rc] = struct{}{}
	t.m.Unlock()
	go rc.readLoop()
	return rc
}

var (
	errUnexpectedResp = errors.New("server misbehaving: unexpected response")
)

func (c *reusableConn) readLoop() {
	for {
		resp, err := dnsutils.ReadRawMsgFromTCP(c.c)
		if err != nil {
			c.closeWithErr(err)
			return
		}

		c.m.Lock()
		respChan := c.waitingResp
		c.waitingResp = nil
		c.m.Unlock()

		if respChan == nil {
			pool.ReleaseBuf(resp)
			c.closeWithErr(errUnexpectedResp)
			return
		}

		// This connection is idled again.
		c.c.SetReadDeadline(time.Now().Add(c.t.idleTimeout))
		// Note: calling setIdle before sending resp back to make sure this connection is idle
		// before Exchange call returning. Otherwise, Test_ReuseConnTransport may fail.
		c.t.setIdle(c)

		select {
		case respChan <- resp:
		default:
			panic("bug: respChan has buffer, we shouldn't reach here")
		}
	}
}

func (c *reusableConn) closeWithErr(err error) {
	if err == nil {
		err = net.ErrClosed
	}
	c.closeOnce.Do(func() {
		c.t.m.Lock()
		delete(c.t.conns, c)
		delete(c.t.idleConns, c)
		c.t.m.Unlock()

		c.closeErr = err
		c.c.Close()
		close(c.closeNotify)
	})
}

func (c *reusableConn) closeWithErrByTransport(err error) {
	if err == nil {
		err = net.ErrClosed
	}
	c.closeOnce.Do(func() {
		c.closeErr = err
		c.c.Close()
		close(c.closeNotify)
	})
}

func (c *reusableConn) exchange(ctx context.Context, q *[]byte) (*[]byte, error) {
	respChan := make(chan *[]byte, 1)
	c.m.Lock()
	if c.waitingResp != nil {
		c.m.Unlock()
		panic("bug: reusableConn: concurrent exchange calls")
	}
	c.waitingResp = respChan
	c.m.Unlock()

	waitRespTimeout := reuseConnQueryTimeout
	if c.t.testWaitRespTimeout > 0 {
		waitRespTimeout = c.t.testWaitRespTimeout
	}
	c.c.SetDeadline(time.Now().Add(waitRespTimeout))
	_, err := c.c.Write(*q)
	if err != nil {
		c.closeWithErr(err)
		return nil, err
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case <-c.closeNotify:
		return nil, c.closeErr
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}
