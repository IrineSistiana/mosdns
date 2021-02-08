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

package fastforward

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
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
)

type transport struct {
	// cannot be nil
	logger      *zap.Logger
	dialFunc    func() (net.Conn, error)
	writeFunc   func(c io.Writer, m *dns.Msg) (n int, err error)
	readFunc    func(c io.Reader) (m *dns.Msg, n int, err error)
	maxConn     int           // including dialing connections. Must >= 1.
	idleTimeout time.Duration // if idleTimeout <=0, transport will not reuse connections.
	readTimeout time.Duration // must > 0

	cm           sync.Mutex // protect the following maps
	conns        map[*dnsConn]struct{}
	dialingCalls map[*dialCall]struct{}

	// for debugging and testing
	connOpened uint32
}

func newTransport(
	logger *zap.Logger,
	dialFunc func() (net.Conn, error),
	writeFunc func(c io.Writer, m *dns.Msg) (n int, err error),
	readFunc func(c io.Reader) (m *dns.Msg, n int, err error),
	maxConn int,
	idleTimeout time.Duration,
	readTimeout time.Duration,
) *transport {
	return &transport{
		logger:      logger,
		dialFunc:    dialFunc,
		writeFunc:   writeFunc,
		readFunc:    readFunc,
		maxConn:     maxConn,
		idleTimeout: idleTimeout,
		readTimeout: readTimeout,

		conns:        make(map[*dnsConn]struct{}),
		dialingCalls: make(map[*dialCall]struct{}),
	}
}

type dialCall struct {
	done chan struct{}
	c    *dnsConn // will be ready after done is closed.
	err  error
}

func (t *transport) exchange(q *dns.Msg) (r *dns.Msg, reusedConn bool, err error) {
	if t.idleTimeout <= 0 {
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

func (t *transport) exchangeNoKeepAlive(q *dns.Msg) (*dns.Msg, error) {
	atomic.AddUint32(&t.connOpened, 1)
	conn, err := t.dialFunc()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(t.readTimeout))
	_, err = t.writeFunc(conn, q)
	if err != nil {
		return nil, err
	}
	r, _, err := t.readFunc(conn)
	return r, err
}

func (t *transport) removeConn(conn *dnsConn) {
	t.cm.Lock()
	delete(t.conns, conn)
	t.cm.Unlock()
}

func (t *transport) getConn() (conn *dnsConn, reusedConn bool, err error) {
	t.cm.Lock()
	for c := range t.conns {
		t.cm.Unlock()
		return c, true, nil
	}

	// need a new connection
	var dCall *dialCall
	if len(t.dialingCalls) < t.maxConn { // we can dial a new connection
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

	timer := utils.GetTimer(t.readTimeout)
	defer utils.ReleaseTimer(timer)
	select {
	case <-timer.C:
		return nil, false, errDialTimeout
	case <-dCall.done:
		return dCall.c, false, dCall.err
	}
}

// startDial: It must be called when t.cm is locked.
func (t *transport) startDial() *dialCall {
	dCall := new(dialCall)
	dCall.done = make(chan struct{})
	t.dialingCalls[dCall] = struct{}{} // add it to dialingCalls

	go func() {
		atomic.AddUint32(&t.connOpened, 1)
		c, err := t.dialFunc()
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

type dnsConn struct {
	t *transport

	c      net.Conn
	nextId uint32 // atomic access

	qm    sync.RWMutex
	queue map[uint16]chan *dns.Msg

	cleanOnce sync.Once
	closeChan chan struct{}
	err       error // will be ready after dnsConn is closed
}

func newDnsConn(t *transport, c net.Conn) *dnsConn {
	return &dnsConn{
		t:         t,
		c:         c,
		queue:     make(map[uint16]chan *dns.Msg),
		closeChan: make(chan struct{}),
	}
}

func (c *dnsConn) preprocess() (id uint16, resChan <-chan *dns.Msg) {
	id = uint16(atomic.AddUint32(&c.nextId, 1))
	rc := make(chan *dns.Msg, 1)
	c.qm.Lock()
	c.queue[id] = rc
	c.qm.Unlock()
	return id, rc
}

func (c *dnsConn) exchange(m *dns.Msg) (r *dns.Msg, err error) {
	queryId, resChan := c.preprocess()
	defer func() {
		c.qm.Lock()
		delete(c.queue, queryId)
		c.qm.Unlock()
	}()

	nm := new(dns.Msg)
	*nm = *m // shadow copy msg, we just need to change its id.
	nm.Id = queryId

	c.c.SetDeadline(time.Now().Add(c.t.idleTimeout))
	_, err = c.t.writeFunc(c.c, nm)
	if err != nil {
		c.closeAndCleanup(err) // abort this connection.
		return nil, err
	}

	timer := utils.GetTimer(c.t.readTimeout)
	defer utils.ReleaseTimer(timer)

	select {
	case <-timer.C:
		return nil, errReadTimeout
	case r := <-resChan:
		r.Id = m.Id
		return r, nil
	case <-c.closeChan:
		return nil, c.err
	}
}

func (c *dnsConn) notifyExchange(m *dns.Msg) {
	c.qm.RLock()
	resChan, ok := c.queue[m.Id]
	c.qm.RUnlock()
	if ok {
		select {
		case resChan <- m:
		default:
		}
	}
}

func (c *dnsConn) readLoop() {
	for {
		c.c.SetDeadline(time.Now().Add(c.t.idleTimeout))
		m, _, err := c.t.readFunc(c.c)
		if m != nil {
			c.notifyExchange(m)
		}
		if err != nil {
			c.closeAndCleanup(err) // abort this connection.
			return
		}
	}
}

func (c *dnsConn) closeAndCleanup(err error) {
	c.cleanOnce.Do(func() {
		c.err = err
		c.t.removeConn(c)
		c.c.Close()
		c.t.logger.Debug(
			"dnsConn read loop exited",
			zap.Stringer("LocalAddr", c.c.LocalAddr()),
			zap.Stringer("RemoteAddr", c.c.RemoteAddr()),
			zap.Error(err),
		)
		close(c.closeChan)
	})
}
