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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/pool"
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

var (
	defaultDialTimeout     = time.Second * 5
	defaultMaxConns        = 1
	defaultMaxQueryPerConn = uint16(65535)
)

type Transport struct {
	// Nil logger disables logging.
	Logger *zap.Logger

	// The following funcs cannot be nil.
	DialFunc  func(ctx context.Context) (net.Conn, error)
	WriteFunc func(c io.Writer, m []byte) (int, error)
	ReadFunc  func(c io.Reader) (*pool.Buffer, int, error)

	// DialTimeout specifies the timeout for DialFunc.
	// Default is defaultDialTimeout.
	DialTimeout time.Duration

	// MaxConns controls the maximum connections Transport can open.
	// It includes dialing connections.
	// Default is 1.
	MaxConns int

	// MaxQueryPerConn controls the maximum queries that one connection
	// handled. The connection will be closed if it reached the limit.
	// Default is 65535.
	MaxQueryPerConn uint16

	// IdleTimeout controls the maximum idle time for each connection.
	// If IdleTimeout <= 0, Transport will not reuse connections.
	IdleTimeout time.Duration

	cm          sync.Mutex // protect the following lazy init fields
	clientConns map[*clientConn]struct{}
	dCalls      map[*dialCall]struct{}
}

func (t *Transport) logger() *zap.Logger {
	if l := t.Logger; l != nil {
		return l
	}
	return nopLogger
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

func (t *Transport) ExchangeContext(ctx context.Context, q []byte) (*pool.Buffer, error) {
	if t.IdleTimeout <= 0 { // no keep alive
		return t.exchangeNoKeepAlive(ctx, q)
	}

	start := time.Now()
	retry := 0
	for {
		conn, reusedConn, qId, err := t.getConn(ctx)
		if err != nil {
			return nil, fmt.Errorf("no available connection, %w", err)
		}

		if !reusedConn {
			return conn.exchange(ctx, q, qId)
		}

		r, err := conn.exchange(ctx, q, qId)
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

func (t *Transport) CloseIdleConnections() {
	t.cm.Lock()
	defer t.cm.Unlock()

	for conn := range t.clientConns {
		if conn.onGoingQuery() == 0 {
			conn.closeAndCleanup(errEOL)
		}
	}
}

func (t *Transport) exchangeNoKeepAlive(ctx context.Context, q []byte) (*pool.Buffer, error) {
	conn, err := t.DialFunc(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_, err = t.WriteFunc(conn, q)
	if err != nil {
		return nil, err
	}

	type result struct {
		b   *pool.Buffer
		err error
	}

	resChan := make(chan *result)
	go func() {
		b, _, err := t.ReadFunc(conn)
		res := &result{b, err}
		select {
		case resChan <- res:
		case <-ctx.Done():
			if b != nil {
				b.Release()
			}
		}
	}()

	select {
	case res := <-resChan:
		return res.b, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *Transport) removeConn(conn *clientConn) {
	t.cm.Lock()
	delete(t.clientConns, conn)
	t.cm.Unlock()
}

type dialCall struct {
	waitingQId uint16 // indicates how many queries are there waiting.

	done chan struct{}
	c    *clientConn // will be ready after done is closed.
	err  error
}

func (t *Transport) getConn(ctx context.Context) (conn *clientConn, reusedConn bool, qId uint16, err error) {
	t.cm.Lock()

	var availableConn *clientConn
	for c := range t.clientConns {
		if c.qId >= t.maxQueryPerConn() { // This connection has served too many queries.
			// Note: the connection will close and clean up itself after its last query finished.
			// We can't close it here. Some queries may still on that connection.
			delete(t.clientConns, c)
			continue
		}
		availableConn = c
		break
	}

	if availableConn != nil && availableConn.onGoingQuery() == 0 { // An idle connection.
		availableConn.qId++
		qId = availableConn.qId
		t.cm.Unlock()
		return availableConn, true, qId, nil
	}

	var dCall *dialCall
	if len(t.clientConns)+len(t.dCalls) >= t.maxConns() {
		// We have reached the limit and can't open a new connection.
		if availableConn != nil { // We will reuse the connection.
			availableConn.qId++
			qId = availableConn.qId
			t.cm.Unlock()
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
		dCall = t.asyncDialLocked()
		qId = 0
	}
	t.cm.Unlock()

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

// asyncDialLocked dials server in another goroutine.
// It must be called when t.cm is locked.
func (t *Transport) asyncDialLocked() *dialCall {
	dCall := new(dialCall)
	dCall.done = make(chan struct{})
	if t.dCalls == nil {
		t.dCalls = make(map[*dialCall]struct{})
	}
	t.dCalls[dCall] = struct{}{} // add it to dCalls

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), t.dialTimeout())
		defer cancel()
		c, err := t.DialFunc(ctx)
		if err != nil {
			dCall.err = err
			close(dCall.done)
			t.cm.Lock()
			delete(t.dCalls, dCall)
			t.cm.Unlock()
			return
		}
		dConn := newClientConn(t, c)
		dCall.c = dConn
		close(dCall.done)

		t.cm.Lock()
		delete(t.dCalls, dCall)
		dConn.qId = dCall.waitingQId
		if t.clientConns == nil {
			t.clientConns = make(map[*clientConn]struct{})
		}
		t.clientConns[dConn] = struct{}{} // add dConn to clientConns
		t.cm.Unlock()

		t.logger().Debug("new connection established", zap.Uint32("id", dConn.connId))
		dConn.readLoop() // no need to start a new goroutine
	}()
	return dCall
}

type clientConn struct {
	t   *Transport
	qId uint16 // Managed and protected by t.

	c net.Conn

	qm      sync.RWMutex
	queue   map[uint16]chan *pool.Buffer
	markEOL bool

	cleanOnce sync.Once
	closeChan chan struct{}
	closeErr  error // will be ready after clientConn is closed

	connId uint32 // Only for logging.
}

var connIdCounter uint32

func newClientConn(t *Transport, c net.Conn) *clientConn {
	return &clientConn{
		t:         t,
		c:         c,
		queue:     make(map[uint16]chan *pool.Buffer),
		closeChan: make(chan struct{}),

		connId: atomic.AddUint32(&connIdCounter, 1),
	}
}

func (c *clientConn) exchange(ctx context.Context, q []byte, qId uint16) (*pool.Buffer, error) {
	resChan := make(chan *pool.Buffer, 1)

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
	buf := pool.GetBuf(len(q))
	defer buf.Release()
	b := buf.Bytes()
	copy(b, q)
	setMsgId(b, qId)

	c.c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err := c.t.WriteFunc(c.c, b)
	if err != nil {
		// Write error usually is fatal. Abort and close this connection.
		c.closeAndCleanup(err)
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-resChan:
		setMsgId(r.Bytes(), getMsgId(q))
		return r, nil
	case <-c.closeChan:
		return nil, c.closeErr
	}
}

func (c *clientConn) notifyExchange(r *pool.Buffer) {
	c.qm.RLock()
	resChan, ok := c.queue[getMsgId(r.Bytes())]
	c.qm.RUnlock()
	if ok {
		select {
		case resChan <- r:
		default:
		}
	}
}

func (c *clientConn) readLoop() {
	for {
		c.c.SetReadDeadline(time.Now().Add(c.t.IdleTimeout))
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

func (c *clientConn) closeAndCleanup(err error) {
	c.cleanOnce.Do(func() {
		c.t.removeConn(c)
		c.c.Close()
		c.closeErr = err
		close(c.closeChan)

		c.t.logger().Debug("connection closed", zap.Uint32("id", c.connId), zap.Error(err))
	})
}

func (c *clientConn) onGoingQuery() int {
	c.qm.RLock()
	defer c.qm.RUnlock()

	return len(c.queue)
}
