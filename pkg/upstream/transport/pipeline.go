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
	"math/rand"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// PipelineTransport will pipeline queries as RFC 7766 6.2.1.1 suggested.
// It also can reuse udp socket. Since dns over udp is some kind of "pipeline".
type PipelineTransport struct {
	PipelineOpts

	m           sync.Mutex // protect following fields
	closed      bool
	r           *rand.Rand
	activeConns []*pipelineConn
	conns       map[*pipelineConn]struct{}
}

type PipelineOpts struct {
	IOOpts

	// MaxConns controls the maximum pipeline connections Transport can open.
	// It includes dialing connections.
	// Default is defaultPipelineMaxConns.
	// Users that have heavy traffic flow should consider to increase
	// this for better load-balancing and latency.
	MaxConn int
}

type pipelineConn struct {
	dc *dnsConn
	wg sync.WaitGroup

	// Note: this field is protected by PipelineTransport.m.
	servedLocked uint16
}

func newPipelineConn(c *dnsConn) *pipelineConn {
	return &pipelineConn{dc: c}
}

func NewPipelineTransport(opt PipelineOpts) *PipelineTransport {
	return &PipelineTransport{
		PipelineOpts: opt,
		r:            rand.New(rand.NewSource(time.Now().Unix())),
		conns:        make(map[*pipelineConn]struct{}),
	}
}

func (t *PipelineTransport) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	const maxAttempt = 3
	attempt := 0
	for {
		pc, allocatedQid, isNewConn, err := t.getPipelineConn()
		if err != nil {
			return nil, err
		}

		r, err := pc.dc.exchangePipeline(ctx, m, allocatedQid)
		pc.wg.Done()

		if err != nil {
			// Reused connection may not stable.
			// Try to re-send this query if it failed on a reused connection.
			if !isNewConn && attempt < maxAttempt && ctx.Err() == nil {
				attempt++
				continue
			}
			return nil, err
		}
		return r, nil
	}
}

// Close closes PipelineTransport and all its connections.
// It always returns a nil error.
func (t *PipelineTransport) Close() error {
	t.m.Lock()
	defer t.m.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	for conn := range t.conns {
		conn.dc.closeWithErr(errClosedTransport)
	}
	return nil
}

// getPipelineConn returns a dnsConn for pipelining queries.
// Caller must call wg.Done() after dnsConn.exchangePipeline.
// The returned dnsConn is ready to serve queries.
func (t *PipelineTransport) getPipelineConn() (
	pc *pipelineConn,
	allocatedQid uint16,
	isNewConn bool,
	err error,
) {
	t.m.Lock()
	if t.closed {
		err = errClosedTransport
		t.m.Unlock()
		return
	}

	pci, pc := t.pickPipelineConnLocked()

	// Dial a new connection if (conn pool is empty), or
	// (the picked conn is busy, and we are allowed to dial more connections).
	maxConn := t.MaxConn
	if maxConn <= 0 {
		maxConn = defaultPipelineMaxConns
	}
	if pc == nil || (pc.dc.queueLen() > pipelineBusyQueueLen && len(t.activeConns) < maxConn) {
		dc := newDnsConn(t.IOOpts)
		pc = newPipelineConn(dc)
		isNewConn = true
		pci = sliceAdd(&t.activeConns, pc)
		t.conns[pc] = struct{}{}
	}

	pc.wg.Add(1)
	pc.servedLocked++
	eol := pc.servedLocked == 65535
	allocatedQid = pc.servedLocked
	if eol {
		// This connection has served too many queries.
		// Note: the connection should be closed only after all its queries finished.
		// We can't close it here. Some queries may still on that connection.
		sliceDel(&t.activeConns, pci) // remove from active conns
	}
	t.m.Unlock()

	if eol {
		// Cleanup when all queries is finished.
		go func() {
			pc.wg.Wait()
			pc.dc.closeWithErr(errEOL)
			t.m.Lock()
			delete(t.conns, pc)
			t.m.Unlock()
		}()
	}
	return
}

// pickPipelineConn picks up a random alive pipelineConn from pool.
// If pool is empty, it returns nil.
// Require holding PipelineTransport.m.
func (t *PipelineTransport) pickPipelineConnLocked() (int, *pipelineConn) {
	for {
		pci, pc := sliceRandGet(t.activeConns, t.r)
		if pc != nil && pc.dc.isClosed() { // closed conn, delete it and retry
			sliceDel(&t.activeConns, pci)
			delete(t.conns, pc)
			continue
		}
		return pci, pc // conn pool is empty or we got a pc
	}
}
