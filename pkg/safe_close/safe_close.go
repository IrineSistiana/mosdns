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

package safe_close

import "sync"

// SafeClose can achieve safe close where WaitClosed returns only after
// all sub goroutines exited.
//
//  1. Main service goroutine starts and wait on ReceiveCloseSignal.
//  2. Any service's sub goroutine should be started by Attach and wait on ReceiveCloseSignal.
//  3. If any fatal err occurs, any service goroutine can call SendCloseSignal to close the service.
//  4. Any third party caller can call SendCloseSignal to close the service.
type SafeClose struct {
	m           sync.Mutex
	wg          sync.WaitGroup
	closeSignal chan struct{}
	closeErr    error
}

func NewSafeClose() *SafeClose {
	return &SafeClose{
		closeSignal: make(chan struct{}),
	}
}

// WaitClosed waits until all SendCloseSignal is called and all
// attached funcs in SafeClose are done.
func (s *SafeClose) WaitClosed() error {
	<-s.closeSignal
	s.wg.Wait()
	return s.closeErr
}

// SendCloseSignal sends a close signal. Unblock WaitClosed.
// The given error will be read by WaitClosed.
// Once SendCloseSignal is called, following calls are noop.
func (s *SafeClose) SendCloseSignal(err error) {
	s.m.Lock()
	select {
	case <-s.closeSignal:
	default:
		s.closeErr = err
		close(s.closeSignal)
	}
	s.m.Unlock()
}

func (s *SafeClose) ReceiveCloseSignal() <-chan struct{} {
	return s.closeSignal
}

// Attach add this goroutine to s.wg WaitClosed.
// f must receive closeSignal and call done when it is done.
// If s was closed, f will not run.
func (s *SafeClose) Attach(f func(done func(), closeSignal <-chan struct{})) {
	s.m.Lock()
	select {
	case <-s.closeSignal:
	default:
		s.wg.Add(1)
		go func() {
			f(s.wg.Done, s.closeSignal)
		}()
	}
	s.m.Unlock()
}
