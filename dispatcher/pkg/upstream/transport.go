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
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/pool"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errDialTimeout = errors.New("dial timeout")
	errReadTimeout = errors.New("read timeout")
	errIdCollision = errors.New("id collision")
)

const (
	defaultReadTimeout = time.Second * 5
	defaultMaxConns    = 1
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
	done chan struct{}
	c    *clientConn // will be ready after done is closed.
	err  error
}

func (t *Transport) Exchange(q *dns.Msg) (r *dns.Msg, reusedConn bool, err error) {
	t.initOnce.Do(t.init)

	if t.IdleTimeout <= 0 {
		r, err = t.exchangeNoKeepAlive(q)
		return r, false, err
	}

	conn, reusedConn, err := t.getConn()
	if err != nil {
		return nil, false, fmt.Errorf("no connection available, %w", err)
	}

	r, err = conn.exchange(q)
	return r, reusedConn, err
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

func (t *Transport) getConn() (conn *clientConn, reusedConn bool, err error) {
	t.cm.Lock()
	for c := range t.conns {
		t.cm.Unlock()
		return c, true, nil
	}

	// need a new connection
	var dCall *dialCall
	if len(t.dialingCalls) < t.maxConns() { // we can dial a new connection
		dCall = t.startDial()
	} else {
		for call := range t.dialingCalls {
			dCall = call
			break
		}
	}
	t.cm.Unlock()

	if dCall == nil {
		panic("Transport getConn: dCall is nil")
	}

	timer := pool.GetTimer(t.timeout())
	defer pool.ReleaseTimer(timer)
	select {
	case <-timer.C:
		return nil, false, errDialTimeout
	case <-dCall.done:
		return dCall.c, false, dCall.err
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
		dConn := newDnsConn(t, c)
		dCall.c = dConn
		close(dCall.done)
		t.cm.Lock()
		t.conns[dConn] = struct{}{} // add dConn to conns
		delete(t.dialingCalls, dCall)
		t.cm.Unlock()

		dConn.readLoop() // no needs to start a new goroutine
	}()
	return dCall
}

type clientConn struct {
	t *Transport

	c net.Conn

	qm    sync.RWMutex
	queue map[uint16]chan *dns.Msg

	cleanOnce sync.Once
	closeChan chan struct{}
	closeErr  error // will be ready after clientConn is closed

	qId uint32 // atomic
}

func newDnsConn(t *Transport, c net.Conn) *clientConn {
	return &clientConn{
		t:         t,
		c:         c,
		queue:     make(map[uint16]chan *dns.Msg),
		closeChan: make(chan struct{}),
	}
}

func (c *clientConn) exchange(qOld *dns.Msg) (*dns.Msg, error) {
	qId := uint16(atomic.AddUint32(&c.qId, 1))
	q := new(dns.Msg)
	*q = *qOld
	q.Id = qId

	resChan := make(chan *dns.Msg, 1)
	c.qm.Lock()
	if _, ok := c.queue[qId]; ok {
		c.qm.Unlock()
		return nil, errIdCollision
	}
	c.queue[qId] = resChan
	c.qm.Unlock()

	defer func() {
		c.qm.Lock()
		delete(c.queue, qId)
		c.qm.Unlock()
	}()

	c.c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err := c.t.WriteFunc(c.c, q)
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
			r.Id = qOld.Id
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
		c.t.logger.Debug(
			"clientConn closed",
			zap.Stringer("LocalAddr", c.c.LocalAddr()),
			zap.Stringer("RemoteAddr", c.c.RemoteAddr()),
			zap.Error(err),
		)
	})
}
