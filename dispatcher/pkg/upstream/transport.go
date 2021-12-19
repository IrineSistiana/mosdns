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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/pool"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"time"
)

var (
	errDialTimeout   = errors.New("dial timeout")
	errReadTimeout   = errors.New("read timeout")
	errIdCollision   = errors.New("id collision")
	errEOL           = errors.New("end of life")
	errConnExhausted = errors.New("connection exhausted")
)

var (
	defaultReadTimeout     = time.Second * 5
	defaultMaxConns        = 1
	defaultMaxQueryPerConn = uint16(65535)
)

type Transport struct {
	// Nil logger disables logging.
	Logger *zap.Logger

	// The following funcs cannot be nil.
	DialFunc  func() (net.Conn, error)
	WriteFunc func(c io.Writer, m *dns.Msg) (n int, err error)
	ReadFunc  func(c io.Reader) (m *dns.Msg, n int, err error)
	// MaxConns controls the maximum connections Transport can open.
	// It includes dialing connections.
	// Default is 1.
	MaxConns int

	// MaxConns controls the maximum queries that one connection
	// handled. The connection will be closed if it reached the limit.
	// Default is 65535.
	MaxQueryPerConn uint16

	// IdleTimeout controls the maximum idle time for each connection.
	// If IdleTimeout <= 0, Transport will not reuse connections.
	IdleTimeout time.Duration

	// Timeout controls the read timeout for each read operation.
	// Default is defaultReadTimeout.
	Timeout time.Duration

	initOnce sync.Once
	logger   *zap.Logger // a non-nil logger

	cm           sync.Mutex // protect the following maps
	conns        map[*clientConn]struct{}
	dialingCalls map[*dialCall]struct{}
}

func (t *Transport) timeout() time.Duration {
	if t := t.Timeout; t > 0 {
		return t
	}
	return defaultReadTimeout
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

func (t *Transport) init() {
	if logger := t.Logger; logger != nil {
		t.logger = logger
	} else {
		t.logger = zap.NewNop()
	}

	t.conns = make(map[*clientConn]struct{})
	t.dialingCalls = make(map[*dialCall]struct{})
}

type dialCall struct {
	waitingQId uint16
	done       chan struct{}
	c          *clientConn // will be ready after done is closed.
	err        error
}

func (t *Transport) Exchange(q *dns.Msg) (r *dns.Msg, err error) {
	t.initOnce.Do(t.init)

	if t.IdleTimeout <= 0 { // no keep alive
		return t.exchangeNoKeepAlive(q)
	}

	start := time.Now()
	retry := 0
	for {
		conn, reusedConn, qId, err := t.getConn()
		if err != nil {
			return nil, fmt.Errorf("no available connection, %w", err)
		}

		if !reusedConn {
			return conn.exchange(q, qId)
		}

		r, err = conn.exchange(q, qId)
		if err != nil {
			if time.Since(start) < time.Millisecond*200 && retry <= 1 {
				retry++
				continue
			}
			return nil, err
		}
		return r, nil
	}
}

func (t *Transport) exchangeNoKeepAlive(q *dns.Msg) (*dns.Msg, error) {
	conn, err := t.DialFunc()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(t.timeout()))
	_, err = t.WriteFunc(conn, q)
	if err != nil {
		return nil, err
	}
	r, _, err := t.ReadFunc(conn)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (t *Transport) removeConn(conn *clientConn) {
	t.cm.Lock()
	delete(t.conns, conn)
	t.cm.Unlock()
}

func (t *Transport) getConn() (conn *clientConn, reusedConn bool, qId uint16, err error) {
	t.cm.Lock()
	var availableConn *clientConn
	for c := range t.conns {
		if c.qId >= t.maxQueryPerConn() { // This connection has serverd too many queries.
			// Note: the connection will close and clean
			// up itself after its last query finished.
			// We don't have to close or delete it here.
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
	if len(t.conns)+len(t.dialingCalls) >= t.maxConns() {
		// We have reached the limit and can't open a new connection.
		if availableConn != nil { // We will reuse the connection.
			availableConn.qId++
			qId = availableConn.qId
			t.cm.Unlock()
			return availableConn, true, qId, nil
		}

		// No connection is available. Only dialingCalls.
		// Wait a ongoing dial to complete.
		for call := range t.dialingCalls {
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
		// Dial it now, to enlarge the capacity.
		dCall = t.startDial()
		qId = 0
	}
	t.cm.Unlock()

	if dCall == nil {
		return nil, false, 0, errConnExhausted
	}

	timer := pool.GetTimer(t.timeout())
	defer pool.ReleaseTimer(timer)
	select {
	case <-timer.C:
		return nil, false, 0, errDialTimeout
	case <-dCall.done:
		c := dCall.c
		err := dCall.err
		if err != nil {
			return nil, false, 0, err
		}
		return c, false, qId, nil
	}
}

// startDial: It must be called when t.cm is locked.
func (t *Transport) startDial() *dialCall {
	dCall := new(dialCall)
	dCall.done = make(chan struct{})
	t.dialingCalls[dCall] = struct{}{} // add it to dialingCalls

	go func() {
		c, err := t.DialFunc()
		if err != nil {
			dCall.err = err
			close(dCall.done)
			t.cm.Lock()
			delete(t.dialingCalls, dCall)
			t.cm.Unlock()
			return
		}
		dConn := newClientConn(t, c)
		dCall.c = dConn
		close(dCall.done)
		t.cm.Lock()
		dConn.qId = dCall.waitingQId
		t.conns[dConn] = struct{}{} // add dConn to conns
		delete(t.dialingCalls, dCall)
		t.cm.Unlock()

		dConn.readLoop() // no need to start a new goroutine
	}()
	return dCall
}

type clientConn struct {
	t   *Transport
	qId uint16 // Managed and protected by t.

	c net.Conn

	qm      sync.RWMutex
	queue   map[uint16]chan *dns.Msg
	markEOL bool

	cleanOnce sync.Once
	closeChan chan struct{}
	closeErr  error // will be ready after clientConn is closed
}

func newClientConn(t *Transport, c net.Conn) *clientConn {
	return &clientConn{
		t:         t,
		c:         c,
		queue:     make(map[uint16]chan *dns.Msg),
		closeChan: make(chan struct{}),
	}
}

func (c *clientConn) exchange(q *dns.Msg, qId uint16) (*dns.Msg, error) {
	resChan := make(chan *dns.Msg, 1)
	c.qm.Lock()
	if c.markEOL {
		c.qm.Unlock()
		return nil, errEOL
	}
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

	qWithNewId := new(dns.Msg)
	*qWithNewId = *q
	qWithNewId.Id = qId
	c.c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err := c.t.WriteFunc(c.c, qWithNewId)
	if err != nil {
		c.closeAndCleanup(err) // abort this connection.
		return nil, err
	}

	timer := pool.GetTimer(c.t.timeout())
	defer pool.ReleaseTimer(timer)

	select {
	case <-timer.C:
		return nil, errReadTimeout
	case r := <-resChan:
		if r != nil {
			r.Id = q.Id
		}
		return r, nil
	case <-c.closeChan:
		return nil, c.closeErr
	}
}

func (c *clientConn) notifyExchange(r *dns.Msg) {
	if r == nil {
		return
	}
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
	})
}

// markAsEOL marks this clientConn as end of life.
func (c *clientConn) markAsEOL() {
	c.qm.Lock()
	c.markEOL = true
	c.qm.Unlock()
}

func (c *clientConn) onGoingQuery() int {
	c.qm.RLock()
	defer c.qm.RUnlock()

	return len(c.queue)
}
