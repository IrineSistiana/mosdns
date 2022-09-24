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
	"net/netip"
	"testing"
	"time"
)

func Test_HPClientLimiter(t *testing.T) {
	limiter := NewHPClientLimiter(8)

	for suffix := 0; suffix < 256; suffix++ {
		addr := netip.AddrFrom4([4]byte{0, 0, 0, byte(suffix)})
		for i := 0; i <= 16; i++ {
			ok := limiter.Acquire(addr)

			if i <= 8 && !ok { // if it not reaches the limit but return a false
				t.Fatal()
			}

			if i > 8 && ok { // if it reached the limit but return a true
				t.Fatal()
			}
		}
	}

	limiterLen := func() int {
		s := 0
		for _, shard := range limiter.shards {
			s += len(shard.noLock.m)
		}
		return s
	}

	if limiterLen() != 256 {
		t.Fatal()
	}

	limiter.GC(time.Now().Add(counterIdleTimeout).Add(time.Hour)) // all counter should be cleaned

	if remain := limiterLen(); remain != 0 {
		t.Fatal("gc test failed")
	}
}
