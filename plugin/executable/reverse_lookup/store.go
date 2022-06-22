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

package reverselookup

import (
	"net/netip"
	"sync"
	"time"
)

type store struct {
	sync.RWMutex
	m map[netip.Addr]*elem

	closeOnce sync.Once
	closeChan chan struct{}
}

type elem struct {
	expire time.Time
	d      string
}

func newStore() *store {
	s := &store{
		m:         make(map[netip.Addr]*elem),
		closeChan: make(chan struct{}),
	}

	go func() {
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()

		for {
			select {
			case <-s.closeChan:
				return
			case <-ticker.C:
				s.clean()
			}
		}
	}()

	return s
}

func (s *store) save(domain string, ttl time.Duration, ip ...netip.Addr) {
	if len(ip) == 0 {
		return
	}

	e := &elem{
		expire: time.Now().Add(ttl),
		d:      domain,
	}

	s.Lock()
	for _, addr := range ip {
		s.m[addr] = e
	}
	s.Unlock()
}

func (s *store) lookup(ip netip.Addr) string {
	s.RLock()
	defer s.RUnlock()
	d := s.m[ip]
	if d == nil {
		return ""
	}
	return d.d
}

func (s *store) clean() {
	now := time.Now()
	s.Lock()
	defer s.Unlock()

	for addr, e := range s.m {
		if e.expire.Before(now) {
			delete(s.m, addr)
		}
	}
}

func (s *store) close() {
	s.closeOnce.Do(func() {
		close(s.closeChan)
	})
}
