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
	"fmt"
	"github.com/miekg/dns"
	"io"
	"sync"
	"sync/atomic"
)

// dnsConn is a low-level connection for dns.
type dnsConn struct {
	IOOpts

	connMu          sync.Mutex
	closed          atomic.Bool // atomic, for fast check
	connReadyNotify chan struct{}
	c               io.ReadWriteCloser // c is ready (not nil) when connReadyNotify is closed.
	cIdleTimer      *idleTimer

	closeNotify chan struct{}
	closeErr    error // closeErr is ready (not nil) when closeNotify is closed.

	queueMu sync.RWMutex
	queue   map[uint16]chan *dns.Msg

	// statWaitingReply indicates this dnsConn is waiting a reply from the peer.
	// It can identify c is dead or buggy in some circumstances. e.g. Network is dropped
	// and the sockets were still open because no fin or rst was received.
	statWaitingReply atomic.Bool
}

func newDnsConn(opt IOOpts) *dnsConn {
	dc := &dnsConn{
		IOOpts:          opt,
		connReadyNotify: make(chan struct{}),
		closeNotify:     make(chan struct{}),
		queue:           make(map[uint16]chan *dns.Msg),
	}
	go dc.dialAndRead()
	return dc
}

// exchange sends q out and waits for its reply. It is caller's responsibility
// to ensure that qid is not duplicated among other ongoing exchange calls for
// the same dnsConn.
func (dc *dnsConn) exchange(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	select {
	case <-dc.connReadyNotify:
	case <-dc.closeNotify:
		return nil, dc.closeErr
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	qid := q.Id
	resChan := make(chan *dns.Msg, 1)
	if ok := dc.addQueueC(qid, resChan); !ok {
		return nil, fmt.Errorf("duplicated qid %d", qid)
	}
	defer dc.deleteQueueC(qid)

	// Reminder: Set write deadline here is not very useful to avoid dead connections.
	// Typically, a write operation will time out only if its socket buffer is full.
	// Ser read deadline is enough.
	_, err := dc.WriteFunc(dc.c, q)
	if err != nil {
		// Write error usually is fatal. Abort and close this connection.
		dc.closeWithErr(err)
		return nil, err
	}

	// If a query was sent, server should have a reply (even not for this query) in a short time.
	// This indicates the connection is healthy. Otherwise, this connection might be dead.
	// The Read deadline will be refreshed in dnsConn.readLoop() after every successful read.
	if dc.statWaitingReply.CompareAndSwap(false, true) {
		dc.cIdleTimer.reset(waitingReplyTimeout)
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

func (dc *dnsConn) exchangePipeline(ctx context.Context, q *dns.Msg, allocatedQid uint16) (*dns.Msg, error) {
	qSend := shadowCopy(q)
	qSend.Id = allocatedQid
	r, err := dc.exchange(ctx, qSend)
	if err != nil {
		return nil, err
	}
	r.Id = q.Id
	return r, nil
}

// dialAndRead dials to the upstream and start a read loop.
// This func should be called in a new goroutine.
func (dc *dnsConn) dialAndRead() {
	dialTimeout := dc.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	c, err := dc.DialFunc(dialCtx)
	if err != nil {
		dc.closeWithErr(fmt.Errorf("failed to dial, %w", err))
		return
	}

	dc.connMu.Lock()
	// dnsConn is closed before dial is complete.
	if dc.closed.Load() {
		dc.connMu.Unlock()
		c.Close()
		return
	}
	dc.c = c
	idleTimeout := dc.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultIdleTimeout
	}
	dc.cIdleTimer = newIdleTimer(idleTimeout, func() {
		_ = c.Close()
	})
	close(dc.connReadyNotify)
	dc.connMu.Unlock()

	dc.readLoop()
}

// readLoop reads dnsConn until dnsConn was closed or there was a read error.
func (dc *dnsConn) readLoop() {
	idleTimeout := dc.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultIdleTimeout
	}

	for {
		dc.cIdleTimer.reset(0)
		r, _, err := dc.ReadFunc(dc.c)
		if err != nil {
			dc.closeWithErr(err) // abort this connection.
			return
		}
		dc.statWaitingReply.Store(false)

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
	return dc.closed.Load()
}

// closeWithErr closes dnsConn with an error. The error will be sent
// to the waiting exchange calls.
// Subsequent calls are noop.
func (dc *dnsConn) closeWithErr(err error) {
	dc.connMu.Lock()
	defer dc.connMu.Unlock()

	if dc.closed.Load() {
		return
	}
	dc.closed.Store(true)
	dc.closeErr = err
	close(dc.closeNotify)

	// dc has dialed a connection.
	if dc.c != nil {
		dc.c.Close()
		dc.cIdleTimer.stop()
	}
}

func (dc *dnsConn) queueLen() int {
	dc.queueMu.RLock()
	defer dc.queueMu.RUnlock()
	return len(dc.queue)
}

func (dc *dnsConn) getQueueC(qid uint16) chan<- *dns.Msg {
	dc.queueMu.RLock()
	defer dc.queueMu.RUnlock()
	return dc.queue[qid]
}

// addQueueC adds qid to the queue if qid is not in the queue and returns true.
// Otherwise, it returns false.
func (dc *dnsConn) addQueueC(qid uint16, c chan *dns.Msg) bool {
	dc.queueMu.Lock()
	defer dc.queueMu.Unlock()
	if _, dup := dc.queue[qid]; dup {
		return false
	}
	dc.queue[qid] = c
	return true
}

func (dc *dnsConn) deleteQueueC(qid uint16) {
	dc.queueMu.Lock()
	defer dc.queueMu.Unlock()
	delete(dc.queue, qid)
}
