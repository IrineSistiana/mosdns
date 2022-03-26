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

package upstream

import (
	"context"
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errIdCollision   = errors.New("id collision")
	errEOL           = errors.New("end of life")
	errConnExhausted = errors.New("connection exhausted")
)

const (
	defaultIdleTimeout             = time.Second * 10
	defaultDialTimeout             = time.Second * 5
	defaultNoPipelineQueryTimeout  = time.Second * 5
	defaultNoConnReuseQueryTimeout = time.Second * 5
	defaultMaxConns                = 1
	defaultMaxQueryPerConn         = 65535
)

// Transport is a DNS msg transport that supposes DNS over UDP,TCP,TLS.
// For UDP, it can reuse UDP sockets.
// For TCP and DoT, it implements RFC 7766 and supports pipeline mode and can handle
// out-of-order responses.
type Transport struct {
	// Nil logger disables logging.
	Logger *zap.Logger

	// The following funcs cannot be nil.
	// DialFunc specifies the method to dial a connection to the server.
	DialFunc func(ctx context.Context) (net.Conn, error)
	// WriteFunc specifies the method to write a wire dns msg to the connection
	// opened by the DialFunc.
	WriteFunc func(c io.Writer, m *dns.Msg) (int, error)
	// ReadFunc specifies the method to read a wire dns msg from the connection
	// opened by the DialFunc.
	ReadFunc func(c io.Reader) (*dns.Msg, int, error)

	// DialTimeout specifies the timeout for DialFunc.
	// Default is defaultDialTimeout.
	DialTimeout time.Duration

	// IdleTimeout controls the maximum idle time for each connection.
	// If IdleTimeout < 0, Transport will not reuse connections.
	// Default is defaultIdleTimeout.
	IdleTimeout time.Duration

	// If EnablePipeline is set and IdleTimeout > 0, the Transport will pipeline
	// queries as RFC 7766 6.2.1.1 suggested.
	EnablePipeline bool

	// MaxConns controls the maximum pipeline connections Transport can open.
	// It includes dialing connections.
	// Default is 1.
	// Each connection can handle no more than 65535 queries concurrently.
	// Typically, it is very rare reaching that limit.
	MaxConns int

	// MaxQueryPerConn controls the maximum queries that one pipeline connection
	// can handle. The connection will be closed if it reached the limit.
	// Default is 65535.
	MaxQueryPerConn uint16

	pm     sync.Mutex // protect the following lazy init fields
	pConns map[*pipelineConn]struct{}
	dCalls map[*pipelineDialCall]struct{}

	opm     sync.Mutex // protect the following lazy init fields
	opConns map[*noPipelineConn]struct{}
}

func (t *Transport) logger() *zap.Logger {
	if l := t.Logger; l != nil {
		return l
	}
	return nopLogger
}

func (t *Transport) idleTimeout() time.Duration {
	if t.IdleTimeout == 0 {
		return defaultIdleTimeout
	}
	return t.IdleTimeout
}

func (t *Transport) dialTimeout() time.Duration {
	if t := t.DialTimeout; t > 0 {
		return t
	}
	return defaultDialTimeout
}

func (t *Transport) maxConns() int {
	if n := t.MaxConns; n > 0 {
		return n
	}
	return defaultMaxConns
}

func (t *Transport) maxQueryPerConn() uint16 {
	if n := t.MaxQueryPerConn; n > 0 {
		return n
	}
	return defaultMaxQueryPerConn
}

func (t *Transport) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	if t.idleTimeout() <= 0 {
		return t.exchangeNoConnReuse(ctx, q)
	}

	if t.EnablePipeline {
		return t.exchangePipelineConnReuse(ctx, q)
	}

	return t.exchangeConnReuse(ctx, q)
}

func (t *Transport) CloseIdleConnections() {
	t.pm.Lock()
	for conn := range t.pConns {
		if conn.onGoingQuery() == 0 {
			conn.closeAndCleanup(errEOL)
		}
	}
	t.pm.Unlock()

	t.opm.Lock()
	for conn := range t.opConns {
		conn.close()
		delete(t.opConns, conn)
	}
	t.opm.Unlock()
}

func (t *Transport) exchangePipelineConnReuse(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	start := time.Now()
	retry := 0
	for {
		conn, reusedConn, qId, err := t.getPipelineConn(ctx)
		if err != nil {
			return nil, fmt.Errorf("no available connection, %w", err)
		}

		if !reusedConn {
			return conn.exchange(ctx, m, qId)
		}

		r, err := conn.exchange(ctx, m, qId)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && time.Since(start) < time.Millisecond*200 && retry <= 1 {
				retry++
				continue
			}
			return nil, err
		}
		return r, nil
	}
}

func (t *Transport) exchangeNoConnReuse(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	conn, err := t.DialFunc(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(getContextDeadline(ctx, defaultNoConnReuseQueryTimeout))

	_, err = t.WriteFunc(conn, m)
	if err != nil {
		return nil, err
	}

	type result struct {
		m   *dns.Msg
		err error
	}

	resChan := make(chan *result, 1)
	go func() {
		b, _, err := t.ReadFunc(conn)
		resChan <- &result{b, err}
	}()

	select {
	case res := <-resChan:
		return res.m, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *Transport) exchangeConnReuse(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	type result struct {
		m   *dns.Msg
		err error
	}

	resChan := make(chan result, 1)
	go func() {
		for ctx.Err() == nil {
			c, reused, err := t.getNoPipelineConn()
			if err != nil {
				resChan <- result{m: nil, err: err}
				return
			}

			b, err := c.exchange(q)
			if err != nil {
				c.close()
				if reused {
					continue
				}
				resChan <- result{m: nil, err: err}
				return
			}

			// No err, reuse the connection.
			t.releaseNoPipelineConn(c)
			resChan <- result{m: b, err: nil}
			return
		}
	}()

	select {
	case res := <-resChan:
		return res.m, res.err

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// getNoPipelineConn returns a *noPipelineConn.
// The idle time of *noPipelineConn is still within Transport.IdleTimeout
// but may be unusable.
func (t *Transport) getNoPipelineConn() (c *noPipelineConn, reused bool, err error) {
	// Get a connection from pool.
	t.opm.Lock()
	for c = range t.opConns {
		delete(t.opConns, c)
		if ok := c.stopIdle(); ok {
			t.opm.Unlock()
			return c, true, nil
		} else { // Conn is already dead.
			c.close()
		}
	}
	t.opm.Unlock()

	// Dial a new connection.
	ctx, cancel := context.WithTimeout(context.Background(), t.dialTimeout())
	defer cancel()
	conn, err := t.DialFunc(ctx)
	return newNpConn(t, conn), false, err
}

func (t *Transport) releaseNoPipelineConn(c *noPipelineConn) {
	t.opm.Lock()
	defer t.opm.Unlock()

	if t.opConns == nil {
		t.opConns = make(map[*noPipelineConn]struct{})
	}
	c.startIdle()
	t.opConns[c] = struct{}{}
}

func (t *Transport) removeConn(conn *pipelineConn) {
	t.pm.Lock()
	delete(t.pConns, conn)
	t.pm.Unlock()
}

type pipelineDialCall struct {
	waitingQId uint16 // indicates how many queries are there waiting.

	done chan struct{}
	c    *pipelineConn // will be ready after done is closed.
	err  error
}

func (t *Transport) getPipelineConn(ctx context.Context) (conn *pipelineConn, reusedConn bool, qId uint16, err error) {
	t.pm.Lock()

	var availableConn *pipelineConn
	for c := range t.pConns {
		if c.qId >= t.maxQueryPerConn() { // This connection has served too many queries.
			// Note: the connection will close and clean up itself after its last query finished.
			// We can't close it here. Some queries may still on that connection.
			delete(t.pConns, c)
			continue
		}
		availableConn = c
		break
	}

	if availableConn != nil && availableConn.onGoingQuery() == 0 { // An idle connection.
		availableConn.qId++
		qId = availableConn.qId
		t.pm.Unlock()
		return availableConn, true, qId, nil
	}

	var dCall *pipelineDialCall
	if len(t.pConns)+len(t.dCalls) >= t.maxConns() {
		// We have reached the limit and can't open a new connection.
		if availableConn != nil { // We will reuse the connection.
			availableConn.qId++
			qId = availableConn.qId
			t.pm.Unlock()
			return availableConn, true, qId, nil
		}

		// No connection is available. Only dCalls.
		// Wait an ongoing dial to complete.
		for call := range t.dCalls {
			if call.waitingQId >= t.maxQueryPerConn() { // To many waiting queries
				continue
			}
			call.waitingQId++
			qId = call.waitingQId
			dCall = call
			break
		}
	} else {
		// No idle connection. Still can dial a new connection.
		// Dial it now. More connection, more stability.
		dCall = t.asyncPipelineDialLocked()
		qId = 0
	}
	t.pm.Unlock()

	if dCall == nil {
		return nil, false, 0, errConnExhausted
	}

	select {
	case <-ctx.Done():
		return nil, false, 0, ctx.Err()
	case <-dCall.done:
		c := dCall.c
		err := dCall.err
		if err != nil {
			return nil, false, 0, err
		}
		return c, false, qId, nil
	}
}

// asyncPipelineDialLocked dials server in another goroutine.
// It must be called when t.pm is locked.
func (t *Transport) asyncPipelineDialLocked() *pipelineDialCall {
	dCall := new(pipelineDialCall)
	dCall.done = make(chan struct{})
	if t.dCalls == nil {
		t.dCalls = make(map[*pipelineDialCall]struct{})
	}
	t.dCalls[dCall] = struct{}{} // add it to dCalls

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), t.dialTimeout())
		defer cancel()
		c, err := t.DialFunc(ctx)
		if err != nil {
			dCall.err = err
			close(dCall.done)
			t.pm.Lock()
			delete(t.dCalls, dCall)
			t.pm.Unlock()
			return
		}
		dConn := newPipelineConn(t, c)
		dCall.c = dConn
		close(dCall.done)

		t.pm.Lock()
		delete(t.dCalls, dCall)
		dConn.qId = dCall.waitingQId
		if t.pConns == nil {
			t.pConns = make(map[*pipelineConn]struct{})
		}
		t.pConns[dConn] = struct{}{} // add dConn to pConns
		t.pm.Unlock()

		t.logger().Debug("new connection established", zap.Uint32("id", dConn.connId))
		dConn.readLoop() // no need to start a new goroutine
	}()
	return dCall
}

type pipelineConn struct {
	t   *Transport
	qId uint16 // Managed and protected by t.

	c net.Conn

	qm      sync.RWMutex
	queue   map[uint16]chan *dns.Msg
	markEOL bool

	cleanOnce sync.Once
	closeChan chan struct{}
	closeErr  error // will be ready after pipelineConn is closed

	connId uint32 // Only for logging.
}

var connIdCounter uint32

func newPipelineConn(t *Transport, c net.Conn) *pipelineConn {
	return &pipelineConn{
		t:         t,
		c:         c,
		queue:     make(map[uint16]chan *dns.Msg),
		closeChan: make(chan struct{}),

		connId: atomic.AddUint32(&connIdCounter, 1),
	}
}

func (c *pipelineConn) exchange(ctx context.Context, q *dns.Msg, qId uint16) (*dns.Msg, error) {
	resChan := make(chan *dns.Msg, 1)

	c.qm.Lock()
	if qId >= c.t.maxQueryPerConn() {
		c.markEOL = true
	}
	if _, ok := c.queue[qId]; ok {
		c.qm.Unlock()
		return nil, errIdCollision
	}
	c.queue[qId] = resChan
	c.qm.Unlock()

	defer func() {
		c.qm.Lock()
		delete(c.queue, qId)
		remain := len(c.queue)
		markEOL := c.markEOL
		c.qm.Unlock()

		if markEOL && remain == 0 { // This is the last goroutine.
			c.closeAndCleanup(errEOL)
		}
	}()

	// We have to modify the query ID, but as a writer we cannot modify q directly.
	// We make a copy of q.
	qCopy := shadowCopy(q)
	qCopy.Id = qId
	c.c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err := c.t.WriteFunc(c.c, qCopy)
	if err != nil {
		// Write error usually is fatal. Abort and close this connection.
		c.closeAndCleanup(err)
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-resChan:
		// Change the query id back.
		r.Id = q.Id
		return r, nil
	case <-c.closeChan:
		return nil, c.closeErr
	}
}

func (c *pipelineConn) notifyExchange(r *dns.Msg) {
	c.qm.RLock()
	resChan, ok := c.queue[r.Id]
	c.qm.RUnlock()
	if ok {
		select {
		case resChan <- r:
		default:
		}
	}
}

func (c *pipelineConn) readLoop() {
	for {
		c.c.SetReadDeadline(time.Now().Add(c.t.idleTimeout()))
		m, _, err := c.t.ReadFunc(c.c)
		if err != nil {
			c.closeAndCleanup(err) // abort this connection.
			return
		}
		if m != nil {
			c.notifyExchange(m)
		}
	}
}

func (c *pipelineConn) closeAndCleanup(err error) {
	c.cleanOnce.Do(func() {
		c.t.removeConn(c)
		c.c.Close()
		c.closeErr = err
		close(c.closeChan)

		c.t.logger().Debug("connection closed", zap.Uint32("id", c.connId), zap.Error(err))
	})
}

func (c *pipelineConn) onGoingQuery() int {
	c.qm.RLock()
	defer c.qm.RUnlock()

	return len(c.queue)
}

type noPipelineConn struct {
	t *Transport
	c net.Conn

	m                sync.Mutex
	closed           bool
	idleTimeoutTimer *time.Timer
}

func newNpConn(t *Transport, c net.Conn) *noPipelineConn {
	nc := &noPipelineConn{
		t: t,
		c: c,
	}
	return nc
}

func (nc *noPipelineConn) exchange(m *dns.Msg) (*dns.Msg, error) {
	nc.c.SetDeadline(time.Now().Add(defaultNoPipelineQueryTimeout))
	if _, err := nc.t.WriteFunc(nc.c, m); err != nil {
		return nil, err
	}
	b, _, err := nc.t.ReadFunc(nc.c)
	return b, err
}

func (nc *noPipelineConn) stopIdle() bool {
	nc.m.Lock()
	defer nc.m.Unlock()
	if nc.closed {
		return true
	}
	if nc.idleTimeoutTimer != nil {
		return nc.idleTimeoutTimer.Stop()
	}
	return true
}

func (nc *noPipelineConn) startIdle() {
	nc.m.Lock()
	defer nc.m.Unlock()

	if nc.closed {
		return
	}

	if nc.idleTimeoutTimer != nil {
		nc.idleTimeoutTimer.Reset(nc.t.idleTimeout())
	} else {
		nc.idleTimeoutTimer = time.AfterFunc(nc.t.idleTimeout(), func() {
			nc.close()
		})
	}
}

func (nc *noPipelineConn) close() {
	nc.m.Lock()
	defer nc.m.Unlock()

	if !nc.closed {
		if nc.idleTimeoutTimer != nil {
			nc.idleTimeoutTimer.Stop()
		}
		nc.c.Close()
		nc.closed = true
	}
}
