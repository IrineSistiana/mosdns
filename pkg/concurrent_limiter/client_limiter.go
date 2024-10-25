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
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/concurrent_map"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"net/netip"
	"sync"
	"time"
)

type ClientLimiter interface {
	AcquireToken(addr netip.Addr) bool
}

const (
	counterIdleTimeout = 10 * time.Second
)

type HPLimiterOpts struct {
	Threshold       int
	Interval        time.Duration
	IPv4Mask        int
	IPv6Mask        int
	CleanerInterval time.Duration
}

func (opts *HPLimiterOpts) Init() error {
	if opts.Threshold < 0 {
		panic("client_limiter: negative rate")
	}
	utils.SetDefaultNum(&opts.Interval, time.Second)
	utils.SetDefaultNum(&opts.CleanerInterval, 10*time.Second)

	if m := opts.IPv4Mask; m < 0 || m > 32 {
		return fmt.Errorf("invalid ipv4 mask %d, should be 0~32", m)
	}

	if m := opts.IPv6Mask; m < 0 || m > 128 {
		return fmt.Errorf("invalid ipv6 mask %d, should be 0~128", m)
	}
	utils.SetDefaultNum(&opts.IPv4Mask, 32)
	utils.SetDefaultNum(&opts.IPv6Mask, 48)
	return nil
}

var _ ClientLimiter = (*HPClientLimiter)(nil)

type HPClientLimiter struct {
	opts        HPLimiterOpts
	closeOnce   sync.Once
	closeNotify chan struct{}
	m           *concurrent_map.Map[netAddrHash, *counter]
}

type netAddrHash netip.Addr

func (h netAddrHash) MapHash() int {
	s := 0
	for _, b := range h.As16() {
		s += int(b)
	}
	return s
}

func NewHPClientLimiter(opts HPLimiterOpts) (*HPClientLimiter, error) {
	if err := opts.Init(); err != nil {
		return nil, err
	}
	l := &HPClientLimiter{
		opts:        opts,
		closeNotify: make(chan struct{}),
		m:           concurrent_map.NewMap[netAddrHash, *counter](),
	}

	if opts.CleanerInterval > 0 {
		go l.cleanerLoop()
	}

	return l, nil
}

func (l *HPClientLimiter) cleanerLoop() {
	ticker := time.NewTicker(l.opts.CleanerInterval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			l.GC(now)
		case <-l.closeNotify:
			return
		}
	}
}

func (l *HPClientLimiter) AcquireToken(addr netip.Addr) bool {
	addr = l.ApplyMask(addr).Addr()
	now := time.Now()
	var res bool
	l.m.TestAndSet(netAddrHash(addr), func(key netAddrHash, v *counter, exist bool) (newV *counter, setV, deleteV bool) {
		if !exist {
			v = &counter{startTime: now}
		}
		if v.startTime.Add(l.opts.Interval).Before(now) {
			v.startTime = now
			v.c = 0
		}
		if v.c < l.opts.Threshold {
			v.c++
			res = true
		} else {
			res = false
		}
		return v, !exist, false
	})
	return res
}

func (l *HPClientLimiter) ApplyMask(addr netip.Addr) netip.Prefix {
	switch {
	case addr.Is4():
		return netip.PrefixFrom(addr, l.opts.IPv4Mask).Masked()
	case addr.Is4In6():
		return netip.PrefixFrom(netip.AddrFrom4(addr.As4()), l.opts.IPv4Mask).Masked()
	case addr.Is6():
		return netip.PrefixFrom(addr, l.opts.IPv6Mask).Masked()
	default:
		return netip.Prefix{}
	}
}

func (l *HPClientLimiter) GC(now time.Time) {
	l.m.RangeDo(func(key netAddrHash, v *counter, ok bool) (newV *counter, setV, deleteV bool) {
		if !ok {
			return nil, false, false
		}
		return nil, false, v.startTime.Add(counterIdleTimeout).Before(now)
	})
}

func (l *HPClientLimiter) Close() error {
	l.closeOnce.Do(func() {
		close(l.closeNotify)
	})
	return nil
}

type counter struct {
	c         int
	startTime time.Time
}