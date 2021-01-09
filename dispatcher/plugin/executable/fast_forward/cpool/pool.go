//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mos-chinadns.
//
//     mos-chinadns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mos-chinadns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package cpool

import (
	"container/list"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"go.uber.org/zap"
	"net"
	"sync"
	"time"
)

type Pool struct {
	maxSize         int
	ttl             time.Duration
	cleanerInterval time.Duration
	logger          *zap.Logger

	sync.Mutex
	cleanerStatus uint8
	pool          *list.List
}

type poolElem struct {
	c           net.Conn
	expiredTime time.Time
}

const (
	cleanerOffline uint8 = iota
	cleanerOnline
)

// New returns a new *Pool.
// If size and ttl are <= 0, the Pool will panic.
// If cleanerInterval is <= 0, the pool cleaner won't be used.
func New(size int, ttl, cleanerInterval time.Duration, logger *zap.Logger) *Pool {
	if size <= 0 || ttl <= 0 {
		panic("invalid pool size or ttl")
	}

	return &Pool{
		maxSize:         size,
		ttl:             ttl,
		cleanerInterval: cleanerInterval,
		logger:          logger,
		pool:            list.New(),
		cleanerStatus:   cleanerOffline,
	}
}

// Put stores c into Pool.
func (p *Pool) Put(c net.Conn) {
	var poppedPoolElem *poolElem
	p.Lock()
	if p.pool.Len() >= p.maxSize { // if pool is full, pop it's first(oldest) element.
		e := p.pool.Front()
		poppedPoolElem = e.Value.(*poolElem)
		p.pool.Remove(e)
	}
	pe := &poolElem{c: c, expiredTime: time.Now().Add(p.ttl)}
	p.pool.PushBack(pe)
	p.Unlock()

	if poppedPoolElem != nil {
		poppedPoolElem.c.Close() // release the old connection
	}

	if p.cleanerInterval > 0 {
		p.tryStartCleanerGoroutine()
	}
}

func (p *Pool) popLatest() (pe *poolElem) {
	p.Lock()
	defer p.Unlock()

	e := p.pool.Back()
	if e != nil {
		return p.pool.Remove(e).(*poolElem)
	}
	return nil
}

func (p *Pool) Get() (c net.Conn) {
	now := time.Now()
	for {
		if pe := p.popLatest(); pe != nil {
			if now.After(pe.expiredTime) {
				pe.c.Close() // expired
				continue
			}
			return pe.c
		} else { // pool is empty
			break
		}
	}

	return nil // no available connection in pool
}

func (p *Pool) ConnRemain() int {
	p.Lock()
	defer p.Unlock()

	return p.pool.Len()
}

func (p *Pool) tryStartCleanerGoroutine() {
	p.Lock()
	defer p.Unlock()

	if p.cleanerStatus == cleanerOffline {
		p.cleanerStatus = cleanerOnline
		go p.startCleaner()
	}
}

func (p *Pool) startCleaner() {
	p.logger.Debug("cpool cleaner started")
	defer p.logger.Debug("cpool cleaner exited")

	timer := utils.GetTimer(p.cleanerInterval)
	defer utils.ReleaseTimer(timer)
	for {
		<-timer.C

		p.Lock()
		connCleaned, connRemain, nextExpire := p.clean()
		if connRemain == 0 { // no connection in pool, stop the cleaner
			p.cleanerStatus = cleanerOffline
			p.Unlock()
			return
		}
		p.Unlock()

		var interval time.Duration
		if nextExpire > p.cleanerInterval {
			interval = nextExpire
		} else {
			interval = p.cleanerInterval
		}
		utils.ResetAndDrainTimer(timer, interval)

		if connCleaned > 0 {
			p.logger.Debug("cpool cleaner clean()", zap.Int("remove", connCleaned), zap.Int("remain", connRemain), zap.Duration("interval", interval))
		}
	}
}

// clean cleans old connections. Must be called after Pool is locked.
func (p *Pool) clean() (connRemoved, connRemain int, nextExpire time.Duration) {
	nextExpire = p.ttl
	// remove expired connections
	var next *list.Element // temporarily store e.Next(), which will not available after list.Remove().
	for e := p.pool.Front(); e != nil; e = next {
		next = e.Next()
		pe := e.Value.(*poolElem)
		if d := time.Until(pe.expiredTime); d <= 0 { // expired, release the resources
			connRemoved++
			pe.c.Close()
			p.pool.Remove(e)
		} else {
			if d < nextExpire {
				nextExpire = d
			}
		}
	}

	return connRemoved, p.pool.Len(), nextExpire
}
