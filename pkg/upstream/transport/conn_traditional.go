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
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
)

var (
	ErrTDCTooManyQueries = errors.New("too many queries") // Connection has too many ongoing queries.
	ErrTDCClosed         = errors.New("dns connection closed")
)

var _ DnsConn = (*TraditionalDnsConn)(nil)

// TraditionalDnsConn is a low-level connection for traditional dns protocol, where
// dns frames transport in a single and simple connection. (e.g. udp, tcp, tls)
type TraditionalDnsConn struct {
	c           NetConn
	isTcp       bool
	idleTimeout time.Duration
	maxCq       int

	closeOnce   sync.Once
	closeNotify chan struct{}
	closed      atomic.Bool // atomic, for fast check
	closeErr    error       // closeErr is ready (not nil) when closeNotify is closed.

	queueMu       sync.RWMutex
	reservedQuery int
	nextQid       uint16
	queue         map[uint32]chan *[]byte // uint32 has fast path

	// waitingResp indicates connection is waiting a reply from the peer.
	// It can identify c is dead or buggy in some circumstances. e.g. Network is dropped
	// and the sockets were still open because no fin or rst was received.
	waitingResp atomic.Bool
}

type TraditionalDnsConnOpts struct {
	// Set to true if underlayer connection require a length header.
	// e.g. TCP and DoT.
	WithLengthHeader bool

	// IdleTimeout controls the maximum idle time for each connection.
	// Default is defaultIdleTimeout.
	IdleTimeout time.Duration

	// MaxConcurrentQuery limits the number of maximum concurrent queries
	// in the connection. Default is defaultTdcMaxConcurrentQuery.
	MaxConcurrentQuery int
}

func NewDnsConn(opt TraditionalDnsConnOpts, conn NetConn) *TraditionalDnsConn {
	dc := &TraditionalDnsConn{
		c:           conn,
		isTcp:       opt.WithLengthHeader,
		closeNotify: make(chan struct{}),
		queue:       make(map[uint32]chan *[]byte),
	}
	setDefaultGZ(&dc.idleTimeout, opt.IdleTimeout, defaultIdleTimeout)
	setDefaultGZ(&dc.maxCq, opt.MaxConcurrentQuery, defaultTdcMaxConcurrentQuery)

	go dc.readLoop()
	return dc
}

// exchange sends q out and waits for its reply.
func (dc *TraditionalDnsConn) exchange(ctx context.Context, q []byte) (*[]byte, error) {
	select {
	case <-dc.closeNotify:
		return nil, ErrTDCClosed
	default:
	}

	assignedQid, respChan := dc.addQueueC()
	if respChan == nil {
		return nil, ErrTDCTooManyQueries
	}
	defer dc.deleteQueueC(assignedQid)

	// Reminder: Set write deadline here is not very useful to avoid dead connections.
	// Typically, a write operation will time out only if its socket buffer is full.
	// Ser read deadline is enough.
	err := dc.writeQuery(q, assignedQid)
	if err != nil {
		// Write error usually is fatal. Abort and close this connection.
		dc.CloseWithErr(fmt.Errorf("write err, %w", err))
		return nil, err
	}

	// If a query was sent, server should have a reply (even not for this query) in a short time.
	// This indicates the connection is healthy. Otherwise, this connection might be dead.
	// The Read deadline will be refreshed in DnsConn.readLoop() after every successful read.
	// Note: There has a race condition in this SetReadDeadline() call and the one in
	// readLoop(). It's not a big problem.
	if dc.waitingResp.CompareAndSwap(false, true) {
		dc.c.SetReadDeadline(time.Now().Add(waitingReplyTimeout))
	}

	var resend <-chan time.Time
	if !dc.isTcp {
		ticker := time.NewTicker(time.Second)
		resend = ticker.C
		defer ticker.Stop()
	}

wait:
	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case <-resend:
		err := dc.writeQuery(q, assignedQid)
		if err != nil {
			dc.CloseWithErr(fmt.Errorf("write err, %w", err))
			return nil, err
		}
		goto wait
	case r := <-respChan:
		orgId := binary.BigEndian.Uint16(q)
		binary.BigEndian.PutUint16(*r, orgId)
		return r, nil
	case <-dc.closeNotify:
		return nil, dc.closeErr
	}
}

func (dc *TraditionalDnsConn) writeQuery(q []byte, assignedQid uint16) error {
	var payload *[]byte
	if dc.isTcp {
		var err error
		payload, err = copyMsgWithLenHdr(q)
		if err != nil {
			return err
		}
		binary.BigEndian.PutUint16((*payload)[2:], assignedQid)
	} else {
		payload = copyMsg(q)
		binary.BigEndian.PutUint16(*payload, assignedQid)
	}
	_, err := dc.c.Write(*payload)
	pool.ReleaseBuf(payload)
	return err
}

func (dc *TraditionalDnsConn) readResp() (payload *[]byte, err error) {
	if dc.isTcp {
		return dnsutils.ReadRawMsgFromTCP(dc.c)
	}
	return readMsgUdp(dc.c)
}

// readLoop reads DnsConn until there was a read error.
func (dc *TraditionalDnsConn) readLoop() {

	for {
		dc.c.SetReadDeadline(time.Now().Add(dc.idleTimeout))
		r, err := dc.readResp()
		if err != nil {
			dc.CloseWithErr(fmt.Errorf("read err, %w", err)) // abort this connection.
			return
		}
		dc.waitingResp.Store(false)

		rid := binary.BigEndian.Uint16(*r)
		resChan := dc.getQueueC(rid)
		if resChan != nil {
			select {
			case resChan <- r: // resChan has buffer
			default:
				pool.ReleaseBuf(r)
			}
		} else {
			pool.ReleaseBuf(r)
		}
	}
}

func (dc *TraditionalDnsConn) IsClosed() bool {
	return dc.closed.Load()
}

func (dc *TraditionalDnsConn) Close() error {
	dc.CloseWithErr(ErrTDCClosed)
	return nil
}

// CloseWithErr closes DnsConn with an error. The error will be sent
// to the waiting Exchange calls.
// Subsequent calls are noop.
// Default err is ErrTDCClosed.
func (dc *TraditionalDnsConn) CloseWithErr(err error) {
	if err == nil {
		err = ErrTDCClosed
	}

	dc.closeOnce.Do(func() {
		dc.closed.Store(true)
		dc.closeErr = err
		close(dc.closeNotify)
		dc.c.Close()
	})
}

func (dc *TraditionalDnsConn) getQueueC(qid uint16) chan<- *[]byte {
	dc.queueMu.RLock()
	defer dc.queueMu.RUnlock()
	return dc.queue[uint32(qid)]
}

func (dc *TraditionalDnsConn) queueLen() int {
	dc.queueMu.RLock()
	defer dc.queueMu.RUnlock()
	return len(dc.queue) + dc.reservedQuery
}

// addQueueC assigns a qid and add it to the queue.
// It returns a nil c if queue has too many queries.
// Caller must call deleteQueueC to release the qid in queue.
func (dc *TraditionalDnsConn) addQueueC() (qid uint16, c chan *[]byte) {
	c = make(chan *[]byte)
	dc.queueMu.Lock()
	for i := 0; i < 100; i++ {
		qid = dc.nextQid
		dc.nextQid++
		if _, dup := dc.queue[uint32(qid)]; dup {
			continue
		}
		dc.queue[uint32(qid)] = c
		dc.queueMu.Unlock()
		return qid, c
	}
	dc.queueMu.Unlock()

	// Too many queries in queue. Can't assign qid.
	return 0, nil
}

func (dc *TraditionalDnsConn) deleteQueueC(qid uint16) {
	dc.queueMu.Lock()
	delete(dc.queue, uint32(qid))
	dc.queueMu.Unlock()
}

func (dc *TraditionalDnsConn) ReserveNewQuery() (_ ReservedExchanger, closed bool) {
	if dc.closed.Load() {
		return nil, true
	}

	dc.queueMu.Lock()
	defer dc.queueMu.Unlock()
	if len(dc.queue)+dc.reservedQuery >= dc.maxCq {
		return nil, false
	}
	dc.reservedQuery++
	return (*tdcOneTimeExchanger)(dc), false
}

type tdcOneTimeExchanger TraditionalDnsConn

var _ ReservedExchanger = (*tdcOneTimeExchanger)(nil)

func (ote *tdcOneTimeExchanger) ExchangeReserved(ctx context.Context, q []byte) (resp *[]byte, err error) {
	defer ote.WithdrawReserved()
	return (*TraditionalDnsConn)(ote).exchange(ctx, q)
}

func (ote *tdcOneTimeExchanger) WithdrawReserved() {
	ote.queueMu.Lock()
	ote.reservedQuery--
	ote.queueMu.Unlock()
}
