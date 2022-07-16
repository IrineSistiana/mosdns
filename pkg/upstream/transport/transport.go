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
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errEOL             = errors.New("end of life")
	errClosedTransport = errors.New("transport has been closed")

	nopLogger = zap.NewNop()
)

const (
	defaultIdleTimeout             = time.Second * 10
	defaultDialTimeout             = time.Second * 5
	defaultNoConnReuseQueryTimeout = time.Second * 5
	defaultMaxConns                = 2
	defaultMaxQueryPerConn         = 65535

	writeTimeout        = time.Second
	connTooOldThreshold = time.Millisecond * 500
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
	// Default is defaultMaxConns.
	// Each connection can handle no more than 65535 queries concurrently.
	// Typically, it is very rare reaching that limit.
	MaxConns int

	// MaxQueryPerConn controls the maximum queries that one pipeline connection
	// can handle. The connection will be closed if it reached the limit.
	// Default is defaultMaxQueryPerConn.
	MaxQueryPerConn uint16

	m                  sync.Mutex // protect following fields
	closed             bool
	pipelineConns      map[*dnsConn]struct{}
	idledReusableConns map[*dnsConn]struct{}
	reusableConns      map[*dnsConn]struct{}
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

func (t *Transport) isClosed() bool {
	t.m.Lock()
	closed := t.closed
	t.m.Unlock()
	return closed
}

func (t *Transport) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	if t.isClosed() {
		return nil, errClosedTransport
	}

	if t.idleTimeout() <= 0 {
		return t.exchangeWithoutConnReuse(ctx, q)
	}

	if t.EnablePipeline {
		return t.exchangeWithPipelineConn(ctx, q)
	}

	return t.exchangeWithReusableConn(ctx, q)
}

func (t *Transport) CloseIdleConnections() {
	t.m.Lock()
	defer t.m.Unlock()

	for conn := range t.pipelineConns {
		if conn.queueLen() == 0 {
			delete(t.pipelineConns, conn)
			conn.closeWithErr(errEOL)
		}
	}
	for conn := range t.idledReusableConns {
		conn.closeWithErr(errEOL)
		delete(t.idledReusableConns, conn)
	}
}

// Close closes the Transport and all its active connections.
// All going queries will fail instantly. It always returns nil error.
func (t *Transport) Close() error {
	t.m.Lock()
	defer t.m.Unlock()

	t.closed = true
	for conn := range t.pipelineConns {
		delete(t.pipelineConns, conn)
		conn.closeWithErr(errClosedTransport)
	}
	for conn := range t.reusableConns {
		conn.closeWithErr(errClosedTransport)
		delete(t.reusableConns, conn)
		delete(t.idledReusableConns, conn)
	}
	return nil
}

func (t *Transport) exchangeWithPipelineConn(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	attempt := 0
	var latestErr error
	for {
		attempt++
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if latestErr != nil {
			t.logger().Debug("retrying pipeline connection", zap.NamedError("previous_err", latestErr), zap.Int("attempt", attempt))
		}

		conn, isNewConn, allocatedQid, err := t.getPipelineConn()
		if err != nil {
			return nil, err
		}

		r, err := conn.exchangePipeline(ctx, m, allocatedQid)
		if err != nil {
			if !isNewConn && attempt <= 3 {
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

func (t *Transport) exchangeWithReusableConn(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	attempt := 0
	var latestErr error
	for {
		attempt++
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if latestErr != nil {
			t.logger().Debug("retrying reusable connection", zap.NamedError("previous_err", latestErr), zap.Int("attempt", attempt))
		}

		conn, isNewConn, err := t.getReusableConn()
		if err != nil {
			return nil, err
		}

		r, err := conn.exchangeConnReuse(ctx, m)
		t.releaseReusableConn(conn, err)
		if err != nil {
			if !isNewConn && attempt <= 3 {
				continue
			}
			return nil, err
		}

		return r, nil
	}
}

// getReusableConn returns a *dnsConn.
// The caller must call releaseReusableConn to release the dnsConn.
func (t *Transport) getReusableConn() (c *dnsConn, reused bool, err error) {
	t.m.Lock()
	defer t.m.Unlock()

	if t.closed {
		return nil, false, errClosedTransport
	}

	for c = range t.idledReusableConns {
		delete(t.idledReusableConns, c)
		if c.isClosed() || t.connTooOld(c) {
			delete(t.reusableConns, c)
			continue
		}
		return c, true, nil
	}

	// Dial a new connection.
	c = newDNSConn(t)
	if t.reusableConns == nil {
		t.reusableConns = make(map[*dnsConn]struct{})
	}
	t.reusableConns[c] = struct{}{}

	return c, false, nil
}

// If err != nil, the released dnsConn will be closed instead of
// returning to the conn pool.
func (t *Transport) releaseReusableConn(c *dnsConn, err error) {
	var closeConn bool

	t.m.Lock()
	if err != nil {
		delete(t.reusableConns, c)
	}
	if !t.closed && err == nil {
		if t.idledReusableConns == nil {
			t.idledReusableConns = make(map[*dnsConn]struct{})
		}
		t.idledReusableConns[c] = struct{}{}
	} else {
		closeConn = true
	}
	t.m.Unlock()

	if closeConn {
		c.closeWithErr(err)
	}
}

func (t *Transport) getPipelineConn() (conn *dnsConn, isNewConn bool, allocatedQid uint16, err error) {
	t.m.Lock()
	defer t.m.Unlock()

	if t.closed {
		return nil, false, 0, errClosedTransport
	}

	// Try to get an existing connection.
	for c := range t.pipelineConns {
		if c.isClosed() || t.connTooOld(c) {
			delete(t.pipelineConns, c)
			continue
		}
		conn = c
		break
	}

	// Create a new connection.
	if conn == nil || (conn.queueLen() > 0 && len(t.pipelineConns) < t.maxConns()) {
		conn = newDNSConn(t)
		isNewConn = true
		if t.pipelineConns == nil {
			t.pipelineConns = make(map[*dnsConn]struct{})
		}
		t.pipelineConns[conn] = struct{}{}
	}

	allocatedQid, eol := conn.allocatePipelineQid()
	if eol { // This connection has served too many queries.
		// Note: the connection will close and clean up itself after its last query finished.
		// We can't close it here. Some queries may still on that connection.
		delete(t.pipelineConns, conn)
	}

	return conn, isNewConn, allocatedQid, nil
}

// connTooOld returns true if c's last read time is close to
// its idle deadline.
func (t *Transport) connTooOld(c *dnsConn) bool {
	if ddl := t.idleTimeout() - connTooOldThreshold; ddl > 0 {
		return c.getLastReadTime().Add(ddl).Before(time.Now())
	}
	return false
}

var dnsConnIdCounter uint32

type dnsConn struct {
	connId uint32 // Only for logging.

	t *Transport

	accumulateId uint32 // atomic
	pipelineWg   sync.WaitGroup
	queueMu      sync.Mutex // queue lock
	queue        map[uint16]chan *dns.Msg

	connMu             sync.Mutex
	dialFinishedNotify chan struct{}
	c                  net.Conn
	closed             bool
	closeNotify        chan struct{}
	closeErr           error

	statMu   sync.Mutex
	lastRead time.Time
}

func newDNSConn(t *Transport) *dnsConn {
	dc := &dnsConn{
		t:                  t,
		dialFinishedNotify: make(chan struct{}),
		queue:              make(map[uint16]chan *dns.Msg),
		closeNotify:        make(chan struct{}),
		connId:             atomic.AddUint32(&dnsConnIdCounter, 1),
	}
	go dc.dialAndRead()
	return dc
}

// allocatePipelineQid returns a qid for the next exchangePipeline() call and an eol mark
// indicates the dnsConn is end-of-life (can't' serve more requests).
// Note: exchangePipeline() must be called after allocatePipelineQid().
// allocatePipelineQid() is not concurrent safe and can only be used in Transport.getPipelineConn().
func (dc *dnsConn) allocatePipelineQid() (qid uint16, eol bool) {
	maxQid := uint32(dc.t.maxQueryPerConn())
	id := atomic.AddUint32(&dc.accumulateId, 1)
	if id > maxQid {
		panic("qid overflowed")
	}
	dc.pipelineWg.Add(1)
	eol = id == maxQid
	if eol {
		go func() {
			dc.pipelineWg.Wait()
			dc.closeWithErr(errEOL)
		}()
	}
	return uint16(id), eol
}

func (dc *dnsConn) exchangeConnReuse(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	return dc.exchange(ctx, q)
}

func (dc *dnsConn) exchangePipeline(ctx context.Context, q *dns.Msg, allocatedQid uint16) (*dns.Msg, error) {
	defer dc.pipelineWg.Done()
	qSend := shadowCopy(q)
	qSend.Id = allocatedQid
	r, err := dc.exchange(ctx, qSend)
	if err != nil {
		return nil, err
	}
	r.Id = q.Id
	return r, nil
}

func (dc *dnsConn) exchange(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	select {
	case <-dc.dialFinishedNotify:
	case <-dc.closeNotify:
		return nil, dc.closeErr
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	qid := q.Id
	resChan := make(chan *dns.Msg, 1)
	dc.addQueueC(qid, resChan)
	defer dc.deleteQueueC(qid)

	dc.c.SetWriteDeadline(time.Now().Add(writeTimeout))
	_, err := dc.t.WriteFunc(dc.c, q)
	if err != nil {
		// Write error usually is fatal. Abort and close this connection.
		dc.closeWithErr(err)
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-resChan:
		return r, nil
	case <-dc.closeNotify:
		return nil, dc.closeErr
	}
}

func (dc *dnsConn) dialAndRead() {
	dialCtx, cancel := context.WithTimeout(context.Background(), defaultDialTimeout)
	defer cancel()
	c, err := dc.t.DialFunc(dialCtx)
	if err != nil {
		dc.closeWithErr(err)
		return
	}

	dc.connMu.Lock()
	// dnsConn is closed before dial is complete.
	if dc.closed {
		dc.connMu.Unlock()
		c.Close()
		return
	}
	dc.c = c
	close(dc.dialFinishedNotify)
	dc.connMu.Unlock()

	dc.readLoop()
}

func (dc *dnsConn) readLoop() {
	for {
		dc.c.SetReadDeadline(time.Now().Add(dc.t.idleTimeout()))
		r, _, err := dc.t.ReadFunc(dc.c)
		if err != nil {
			dc.closeWithErr(err) // abort this connection.
			return
		}
		dc.updateReadTime()

		resChan := dc.getQueueC(r.Id)
		if resChan != nil {
			select {
			case resChan <- r: // resChan has buffer
			default:
			}
		}
	}
}

func (dc *dnsConn) isClosed() bool {
	dc.connMu.Lock()
	defer dc.connMu.Unlock()
	return dc.closed
}

func (dc *dnsConn) closeWithErr(err error) {
	dc.connMu.Lock()
	defer dc.connMu.Unlock()

	if dc.closed {
		return
	}
	dc.closed = true
	dc.closeErr = err
	close(dc.closeNotify)

	if dc.c != nil {
		dc.c.Close()
	}

	dc.t.logger().Debug("connection closed", zap.Uint32("id", dc.connId), zap.Error(err))
}

func (dc *dnsConn) queueLen() int {
	dc.queueMu.Lock()
	defer dc.queueMu.Unlock()
	return len(dc.queue)
}

func (dc *dnsConn) getQueueC(qid uint16) chan<- *dns.Msg {
	dc.queueMu.Lock()
	defer dc.queueMu.Unlock()
	return dc.queue[qid]
}

func (dc *dnsConn) addQueueC(qid uint16, c chan *dns.Msg) {
	dc.queueMu.Lock()
	defer dc.queueMu.Unlock()
	dc.queue[qid] = c
}

func (dc *dnsConn) deleteQueueC(qid uint16) {
	dc.queueMu.Lock()
	defer dc.queueMu.Unlock()
	delete(dc.queue, qid)
}

func (dc *dnsConn) updateReadTime() {
	t := time.Now()
	dc.statMu.Lock()
	defer dc.statMu.Unlock()
	dc.lastRead = t
}

func (dc *dnsConn) getLastReadTime() time.Time {
	dc.statMu.Lock()
	defer dc.statMu.Unlock()
	return dc.lastRead
}
