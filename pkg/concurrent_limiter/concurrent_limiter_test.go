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

package concurrent_limiter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test_ConcurrentLimiter(t *testing.T) {
	l := NewConcurrentLimiter(100, 200)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	wg := new(sync.WaitGroup)
	wg.Add(300)
	run := uint32(0)
	for i := 0; i < 300; i++ {
		go func() {
			defer wg.Done()
			select {
			case l.Run() <- struct{}{}:
				atomic.AddUint32(&run, 1)
				defer l.RunDone()
			case <-ctx.Done():
				t.Fail()
			}
		}()
	}

	wg.Wait()
	if l.AvailableRunning() != 100 {
		t.Fatal()
	}
	if run != 300 {
		t.Fatal()
	}

	wg.Add(300)
	allWaitDone := make(chan struct{})
	wdwg := new(sync.WaitGroup)
	wdwg.Add(300)
	discarded := uint32(0)
	for i := 0; i < 300; i++ {
		go func() {
			defer wg.Done()
			if !l.Wait() {
				wdwg.Done()
				atomic.AddUint32(&discarded, 1)
				return
			}
			wdwg.Done()
			<-allWaitDone
			l.WaitDone()
		}()
	}
	wdwg.Wait()
	close(allWaitDone)

	wg.Wait()
	if l.AvailableWaiting() != 200 {
		t.Fatal()
	}
	if discarded != 100 {
		t.Fatal()
	}
}
