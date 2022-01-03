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

package concurrent_limiter

import (
	"sync"
)

// ConcurrentLimiter is a soft limiter.
// It can limit how many tasks are running and track how many tasks
// are going to run.
type ConcurrentLimiter struct {
	maxWaiting int

	wm            sync.Mutex
	waiting       int
	runningBucket chan struct{}
}

// NewConcurrentLimiter returns a ConcurrentLimiter that can limit
// the number of running goroutine to the maxRunning and the number of
// waiting goroutine to maxWaiting.
// maxWaiting >= maxRunning > 0.
func NewConcurrentLimiter(maxRunning, maxWaiting int) *ConcurrentLimiter {
	if !(maxWaiting > 0) || !(maxWaiting >= maxRunning) {
		panic("invalid args")
	}
	return &ConcurrentLimiter{maxWaiting: maxWaiting, runningBucket: make(chan struct{}, maxRunning)}
}

func (l *ConcurrentLimiter) Run() chan<- struct{} {
	return l.runningBucket
}

// RunDone releases the permission.
func (l *ConcurrentLimiter) RunDone() {
	select {
	case <-l.runningBucket:
	default:
		panic("bucket overflow")
	}
}

func (l *ConcurrentLimiter) Wait() bool {
	l.wm.Lock()
	defer l.wm.Unlock()

	if l.waiting >= l.maxWaiting {
		return false
	}
	l.waiting++
	return true
}

func (l *ConcurrentLimiter) WaitDone() {
	l.wm.Lock()
	defer l.wm.Unlock()
	l.waiting--
	if l.maxWaiting <= 0 {
		panic("maxWaiting underflow")
	}
}

func (l *ConcurrentLimiter) AvailableRunning() int {
	return cap(l.runningBucket) - len(l.runningBucket)
}

func (l *ConcurrentLimiter) AvailableWaiting() int {
	l.wm.Lock()
	defer l.wm.Unlock()
	return l.maxWaiting - l.waiting
}

func (l *ConcurrentLimiter) MaxRunning() int {
	return cap(l.runningBucket)
}

func (l *ConcurrentLimiter) MaxWaiting() int {
	return l.maxWaiting
}
