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
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errEOL = errors.New("end of life")
)

const (
	defaultIdleTimeout             = time.Second * 10
	defaultReadTimeout             = time.Second * 5
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

	opm     sync.Mutex // protect the following lazy init fields
	opConns map[*reusableConn]struct{}
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
		return t.exchangeWithoutConnReuse(ctx, q)
	}

	if t.EnablePipeline {
		return t.exchangeWithPipelineConn(ctx, q)
	}

	return t.exchangeWithReusableConn(ctx, q)
}

func (t *Transport) CloseIdleConnections() {
	t.pm.Lock()
	for conn := range t.pConns {
		if conn.queueLen() == 0 {
			delete(t.pConns, conn)
			conn.closeWithErr(errEOL)
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

func (t *Transport) exchangeWithPipelineConn(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	attempt := 0
	for {
		attempt++
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		conn, qid, resChan := t.getPipelineConn()
		reusedConn := conn.dialFinished()

		r, err := conn.exchange(ctx, m, qid, resChan)
		if err != nil {
			if reusedConn && attempt <= 3 {
				t.logger().Debug("retrying pipeline connection", zap.NamedError("previous_err", err), zap.Int("attempt", attempt))
				continue
			}
			return nil, err
		}
		return r, nil
	}
}

func (t *Transport) exchangeWithoutConnReuse(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
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

func (t *Transport) exchangeWithReusableConn(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	type result struct {
		m   *dns.Msg
		err error
	}

	resChan := make(chan result, 1)
	go func() {
		for ctx.Err() == nil {
			c, reused, err := t.getReusableConn()
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
			t.releaseReusableConn(c)
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

// getReusableConn returns a *reusableConn.
// The idle time of *reusableConn is still within Transport.IdleTimeout
// but the inner socket may be unusable (closed, reset, etc.).
func (t *Transport) getReusableConn() (c *reusableConn, reused bool, err error) {
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

func (t *Transport) releaseReusableConn(c *reusableConn) {
	t.opm.Lock()
	defer t.opm.Unlock()

	if t.opConns == nil {
		t.opConns = make(map[*reusableConn]struct{})
	}
	c.startIdle()
	t.opConns[c] = struct{}{}
}

func (t *Transport) getPipelineConn() (conn *pipelineConn, qid uint16, resChan chan *dns.Msg) {
	t.pm.Lock()
	defer t.pm.Unlock()

	// Try to get an existing connection.
	for c := range t.pConns {
		if c.isClosed() {
			delete(t.pConns, conn)
			continue
		}
		conn = c
		break
	}

	// Create a new connection.
	if conn == nil || (conn.queueLen() > 0 && len(t.pConns) < t.maxConns()) {
		conn = newPipelineConn(t)
		if t.pConns == nil {
			t.pConns = make(map[*pipelineConn]struct{})
		}
		t.pConns[conn] = struct{}{}
	}

	qid, resChan, eol := conn.acquireQueueId()
	if eol { // This connection has served too many queries.
		// Note: the connection will close and clean up itself after its last query finished.
		// We can't close it here. Some queries may still on that connection.
		delete(t.pConns, conn)
	}

	return conn, qid, resChan
}

type pipelineConn struct {
	connId uint32 // Only for logging.

	t *Transport

	qm           sync.RWMutex // queue lock
	accumulateId uint16
	eol          bool
	queue        map[uint16]chan *dns.Msg

	cm                 sync.Mutex // connection lock
	dialFinishedNotify chan struct{}
	c                  net.Conn
	dialErr            error
	closeNotify        chan struct{}
	closeErr           error
}

var pipelineConnIdCounter uint32

func newPipelineConn(t *Transport) *pipelineConn {
	dialCtx, cancel := context.WithTimeout(context.Background(), defaultDialTimeout)
	pc := &pipelineConn{
		t: t,

		dialFinishedNotify: make(chan struct{}),
		queue:              make(map[uint16]chan *dns.Msg),
		closeNotify:        make(chan struct{}),

		connId: atomic.AddUint32(&pipelineConnIdCounter, 1),
	}

	go func() {
		defer cancel()
		c, err := t.DialFunc(dialCtx)

		pc.cm.Lock()
		pc.c = c
		pc.dialErr = err
		close(pc.dialFinishedNotify)

		if err != nil { // dial err, close the connection
			if !chanClosed(pc.closeNotify) {
				close(pc.closeNotify)
			}
			pc.cm.Unlock()
			return
		}

		// dial completed.
		// pipelineConn was closed before dial completing.
		if chanClosed(pc.closeNotify) {
			c.Close() // close the sub connection
			pc.cm.Unlock()
			return
		}
		pc.cm.Unlock()

		pc.readLoop()
	}()
	return pc
}

func (c *pipelineConn) acquireQueueId() (qid uint16, resChan chan *dns.Msg, eol bool) {
	resChan = make(chan *dns.Msg, 1)

	c.qm.Lock()
	defer c.qm.Unlock()

	if c.eol {
		panic("invalid acquireQueueId() call, qid overflowed")
	}

	c.accumulateId++
	qid = c.accumulateId
	if qid >= c.t.maxQueryPerConn() {
		eol = true
		c.eol = true
	}
	c.queue[qid] = resChan
	return qid, resChan, eol
}

func (c *pipelineConn) dialFinished() bool {
	return chanClosed(c.dialFinishedNotify)
}

func (c *pipelineConn) exchange(
	ctx context.Context,
	q *dns.Msg,
	qid uint16,
	resChan chan *dns.Msg,
) (*dns.Msg, error) {

	// Release qid and close the connection if it's eol.
	defer func() {
		c.qm.Lock()
		defer c.qm.Unlock()

		delete(c.queue, qid)
		if c.eol && len(c.queue) == 0 { // last query
			c.closeWithErr(errEOL)
		}
	}()

	select {
	case <-c.dialFinishedNotify:
	case <-c.closeNotify:
		return nil, c.closeErr
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if c.dialErr != nil {
		return nil, c.dialErr
	}

	// We have to modify the query ID, but as a writer we cannot modify q directly.
	// We make a copy of q.
	qCopy := shadowCopy(q)
	qCopy.Id = qid
	c.c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))

	// Set read ddl only for the first request.
	// The ddl for the following requests will be set and updated in the
	// read loop.
	if c.queueLen() == 1 {
		c.c.SetReadDeadline(time.Now().Add(defaultReadTimeout))
	}
	_, err := c.t.WriteFunc(c.c, qCopy)
	if err != nil {
		// Write error usually is fatal. Abort and close this connection.
		c.closeWithErr(err)
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-resChan:
		// Change the query id back.
		r.Id = q.Id
		return r, nil
	case <-c.closeNotify:
		return nil, c.closeErr
	}
}

func (c *pipelineConn) readLoop() {
	c.c.SetReadDeadline(time.Now().Add(defaultReadTimeout))
	for {
		r, _, err := c.t.ReadFunc(c.c)
		if err != nil {
			c.closeWithErr(err) // abort this connection.
			return
		}

		c.qm.Lock()
		resChan, ok := c.queue[r.Id]
		if ok {
			delete(c.queue, r.Id)
		}
		queueLen := len(c.queue)
		c.qm.Unlock()

		if ok {
			select {
			case resChan <- r: // resChan has buffer
			default:
			}
		}

		if queueLen > 0 {
			c.c.SetReadDeadline(time.Now().Add(defaultReadTimeout))
		} else {
			c.c.SetReadDeadline(time.Now().Add(c.t.idleTimeout()))
		}
	}
}

func (c *pipelineConn) isClosed() bool {
	return chanClosed(c.closeNotify)
}

func (c *pipelineConn) closeWithErr(err error) {
	c.cm.Lock()
	defer c.cm.Unlock()
	if chanClosed(c.closeNotify) {

		return
	}

	c.closeErr = err
	close(c.closeNotify)

	if c.c != nil {
		c.c.Close()
	}

	c.t.logger().Debug("connection closed", zap.Uint32("id", c.connId), zap.Error(err))
}

func (c *pipelineConn) queueLen() int {
	c.qm.RLock()
	defer c.qm.RUnlock()

	return len(c.queue)
}

type reusableConn struct {
	t *Transport
	c net.Conn

	m                sync.Mutex
	closed           bool
	idleTimeoutTimer *time.Timer
}

func newNpConn(t *Transport, c net.Conn) *reusableConn {
	nc := &reusableConn{
		t: t,
		c: c,
	}
	return nc
}

func (rc *reusableConn) exchange(m *dns.Msg) (*dns.Msg, error) {
	rc.c.SetDeadline(time.Now().Add(defaultNoPipelineQueryTimeout))
	if _, err := rc.t.WriteFunc(rc.c, m); err != nil {
		return nil, err
	}
	b, _, err := rc.t.ReadFunc(rc.c)
	return b, err
}

// If stopIdle returns false, then nc is closed by the
// idle timer
func (rc *reusableConn) stopIdle() bool {
	rc.m.Lock()
	defer rc.m.Unlock()
	if rc.closed {
		return false
	}
	if rc.idleTimeoutTimer != nil {
		return rc.idleTimeoutTimer.Stop()
	}
	return true
}

func (rc *reusableConn) startIdle() {
	rc.m.Lock()
	defer rc.m.Unlock()

	if rc.closed {
		return
	}

	if rc.idleTimeoutTimer != nil {
		rc.idleTimeoutTimer.Reset(rc.t.idleTimeout())
	} else {
		rc.idleTimeoutTimer = time.AfterFunc(rc.t.idleTimeout(), func() {
			rc.close()
		})
	}
}

func (rc *reusableConn) close() {
	rc.m.Lock()
	defer rc.m.Unlock()

	if !rc.closed {
		if rc.idleTimeoutTimer != nil {
			rc.idleTimeoutTimer.Stop()
		}
		rc.c.Close()
		rc.closed = true
	}
}
