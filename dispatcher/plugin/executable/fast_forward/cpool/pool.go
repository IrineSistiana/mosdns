//     Copyright (C) 2020, IrineSistiana
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
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Pool struct {
	maxSize         int
	ttl             time.Duration
	cleanerInterval time.Duration
	logger          *logrus.Entry

	cleanerStatus int32
	sync.Mutex
	pool *list.List
}

type poolElem struct {
	c           net.Conn
	expiredTime time.Time
}

const (
	cleanerOffline int32 = iota
	cleanerOnline
)

func New(size int, ttl, cleanerInterval time.Duration, logger *logrus.Entry) *Pool {
	if cleanerInterval <= 0 {
		panic(fmt.Sprintf("cpool: pool cleaner interval should greater than 0, but got %d", cleanerInterval))
	}

	if size <= 0 || ttl <= 0 {
		return nil
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

func (p *Pool) Put(c net.Conn) {
	if p == nil {
		c.Close()
		return
	}

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

	p.tryStartCleanerGoroutine()
}

func (p *Pool) Get() (c net.Conn) {
	if p == nil {
		return nil
	}

	var pe *poolElem
	p.Lock()
	e := p.pool.Back()
	if e != nil {
		pe = e.Value.(*poolElem)
		p.pool.Remove(e)
	}
	p.Unlock()

	if pe != nil {
		if time.Now().After(pe.expiredTime) {
			pe.c.Close() // expired
			return nil
		}
		return pe.c
	}

	return nil // no available connection in pool
}

func (p *Pool) ConnRemain() int {
	if p == nil {
		return 0
	}

	p.Lock()
	defer p.Unlock()

	return p.pool.Len()
}

func (p *Pool) tryStartCleanerGoroutine() {
	if atomic.CompareAndSwapInt32(&p.cleanerStatus, cleanerOffline, cleanerOnline) {
		go func() {
			p.startCleaner()
			atomic.StoreInt32(&p.cleanerStatus, cleanerOffline)
		}()
	}
}

func (p *Pool) startCleaner() {
	p.logger.Debugf("cpool cleaner %p started", p)
	defer p.logger.Debugf("cpool cleaner %p exited", p)

	timer := utils.GetTimer(p.cleanerInterval)
	defer utils.ReleaseTimer(timer)
	for {
		<-timer.C

		p.Lock()
		connCleaned, connRemain, nextExpire := p.clean()
		if connRemain == 0 { // no connection in pool, stop the cleaner
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
		p.logger.Debugf("cpool cleaner %p removed conn: %d, remain: %d, interval: %.2f", p, connCleaned, connRemain, interval.Seconds())
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
