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
	"encoding/binary"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/miekg/dns"
)

const (
	dnsHeaderLen = 12 // minimum dns msg size
)

func copyMsgWithLenHdr(m []byte) (*[]byte, error) {
	l := len(m)
	if l > dns.MaxMsgSize {
		return nil, ErrPayloadOverFlow
	}
	bp := pool.GetBuf(l + 2)
	binary.BigEndian.PutUint16(*bp, uint16(l))
	copy((*bp)[2:], m)
	return bp, nil
}

func copyMsg(m []byte) *[]byte {
	bp := pool.GetBuf(len(m))
	copy((*bp), m)
	return bp
}

var respChanPool = sync.Pool{
	New: func() any {
		return make(chan *[]byte, 1)
	},
}

func getRespChan() chan *[]byte {
	return respChanPool.Get().(chan *[]byte)
}
func releaseRespChan(c chan *[]byte) {
	select {
	case payload := <-c:
		ReleaseResp(payload)
	default:
	}
	respChanPool.Put(c)
}

// sliceAdd adds v to s and returns its index in s.
func sliceAdd[T any](s *[]T, v T) int {
	*s = append(*s, v)
	return len(*s) - 1
}

// sliceDel deletes the value at index i.
// sliceDel will automatically reduce the cap of the s.
func sliceDel[T any](s *[]T, i int) {
	var zeroT T
	c := cap(*s)
	l := len(*s)

	(*s)[i] = (*s)[l-1]
	(*s)[l-1] = zeroT
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
	d       time.Duration
	m       sync.Mutex
	t       *time.Timer
	stopped bool
}

func newIdleTimer(d time.Duration, f func()) *idleTimer {
	return &idleTimer{
		d: d,
		t: time.AfterFunc(d, f),
	}
}

func (t *idleTimer) reset(d time.Duration) {
	t.m.Lock()
	defer t.m.Unlock()
	if t.stopped {
		return
	}

	if d <= 0 {
		d = t.d
	}

	if !t.t.Reset(d) {
		t.stopped = true
		// re-activated. stop it
		t.t.Stop()
	}
}

func (t *idleTimer) stop() {
	t.m.Lock()
	defer t.m.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true
	t.t.Stop()
}

// readMsgUdp reads dns frame from r. r typically should be a udp connection.
// It uses a 4kb rx buffer and ignores any payload that is too small for a dns msg.
// If no error, the length of payload always >= 12 bytes.
func readMsgUdp(r io.Reader) (*[]byte, error) {
	// TODO: Make this configurable?
	// 4kb should be enough.
	payload := pool.GetBuf(4095)

readAgain:
	n, err := r.Read(*payload)
	if err != nil {
		pool.ReleaseBuf(payload)
		return nil, err
	}
	if n < dnsHeaderLen {
		goto readAgain
	}
	*payload = (*payload)[:n]
	return payload, err
}
