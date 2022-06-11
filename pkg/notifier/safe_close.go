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

package notifier

import "sync"

type SafeClose struct {
	m           sync.Mutex
	wg          sync.WaitGroup
	closeSignal chan struct{}
	done        chan struct{}
	closeErr    error
}

func NewSafeClose() *SafeClose {
	return &SafeClose{
		closeSignal: make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// CloseWait sends a close signal to SafeClose and wait until it is closed.
// It is concurrent safe and can be called multiple times.
// CloseWait blocks until s.Done() is called and all Attach-ed goroutines is done.
func (s *SafeClose) CloseWait() {
	s.m.Lock()
	select {
	case <-s.closeSignal:
	default:
		close(s.closeSignal)
	}
	s.m.Unlock()

	s.wg.Wait()
	<-s.done
}

// SendCloseSignal sends a close signal.
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

// Err returns the first SendCloseSignal error.
func (s *SafeClose) Err() error {
	s.m.Lock()
	defer s.m.Unlock()
	return s.closeErr
}

func (s *SafeClose) ReceiveCloseSignal() <-chan struct{} {
	return s.closeSignal
}

// Attach add this goroutine to s.wg CloseWait.
// f must receive closeSignal and call done when it is done.
// If s was closed, f will not run.
func (s *SafeClose) Attach(f func(done func(), closeSignal <-chan struct{})) {
	s.m.Lock()
	select {
	case <-s.closeSignal:
	default:
		s.wg.Add(1)
		go func() {
			f(s.attachDone, s.closeSignal)
		}()
	}
	s.m.Unlock()
}

func (s *SafeClose) attachDone() {
	s.m.Lock()
	defer s.m.Unlock()
	s.wg.Done()
}

// Done notifies CloseWait that is done.
// It is concurrent safe and can be called multiple times.
func (s *SafeClose) Done() {
	s.m.Lock()
	defer s.m.Unlock()

	select {
	case <-s.done:
	default:
		close(s.done)
	}
}
