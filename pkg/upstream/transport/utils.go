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
	"github.com/miekg/dns"
	"math/rand"
	"sync/atomic"
	"time"
)

func shadowCopy(m *dns.Msg) *dns.Msg {
	nm := new(dns.Msg)
	*nm = *m
	return nm
}

// sliceAdd adds v to s and returns its index in s.
func sliceAdd[T any](s *[]T, v T) int {
	*s = append(*s, v)
	return len(*s) - 1
}

// sliceDel deletes the value at index i.
// sliceDel will automatically reduce the cap of the s.
func sliceDel[T any](s *[]T, i int) {
	c := cap(*s)
	l := len(*s)

	(*s)[i] = (*s)[l-1]
	*s = (*s)[:l-1]
	l--

	// reduce slice cap to 1/2 if its size is smaller than 1/4 of its cap.
	if c > 32 && (c>>2 >= l) {
		*s = append(make([]T, 0, c>>1), *s...)
	}
}

// sliceRandGet randomly gets a value from s and its index.
// It returns -1 if s is empty.
func sliceRandGet[T any](s []T, r *rand.Rand) (int, T) {
	switch len(s) {
	case 0:
		var v T
		return -1, v
	case 1:
		return 0, s[0]
	default:
		i := r.Intn(len(s))
		return i, s[i]
	}
}

// sliceRandPop randomly pops a value from s.
// It returns false if s is empty.
func sliceRandPop[T any](s *[]T, r *rand.Rand) (T, bool) {
	i, v := sliceRandGet(*s, r)
	if i == -1 {
		return v, false
	}
	sliceDel(s, i)
	return v, true
}

// slicePopLatest pops the latest value from s.
// It returns false if s is empty.
func slicePopLatest[T any](s *[]T) (T, bool) {
	if len(*s) == 0 {
		var v T
		return v, false
	}
	i := len(*s) - 1
	v := (*s)[i]
	sliceDel(s, i)
	return v, true
}

type idleTimer struct {
	d        time.Duration
	updating atomic.Bool
	t        *time.Timer
	stopped  bool
}

func newIdleTimer(d time.Duration, f func()) *idleTimer {
	return &idleTimer{
		d: d,
		t: time.AfterFunc(d, f),
	}
}

func (t *idleTimer) reset(d time.Duration) {
	if t.updating.CompareAndSwap(false, true) {
		defer t.updating.Store(false)
		if t.stopped {
			return
		}
		if d <= 0 {
			d = t.d
		}
		if !t.t.Reset(t.d) {
			t.stopped = true
			// re-activated. stop it
			t.t.Stop()
		}
	}
}

func (t *idleTimer) stop() {
	t.t.Stop()
}
