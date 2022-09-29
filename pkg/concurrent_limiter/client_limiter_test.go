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
	limiter, err := NewHPClientLimiter(HPLimiterOpts{
		Threshold: 8,
		IPv4Mask:  24,
	})

	if err != nil {
		t.Fatal(err)
	}

	for suffix := 0; suffix < 256; suffix++ {
		addr := netip.AddrFrom4([4]byte{0, 0, byte(suffix), 0})
		for i := 1; i <= 16; i++ {
			ok := limiter.AcquireToken(addr)

			if i <= 8 && !ok { // if it not reaches the limit but return a false
				t.Fatal()
			}

			if i > 8 && ok { // if it reached the limit but return a true
				t.Fatal()
			}
		}
	}

	if limiter.m.Len() != 256 {
		t.Fatal()
	}

	limiter.GC(time.Now().Add(counterIdleTimeout).Add(time.Hour)) // all counter should be cleaned

	if remain := limiter.m.Len(); remain != 0 {
		t.Fatal("gc test failed")
	}
}

func Benchmark_HPClientLimiter_AcquireToken(b *testing.B) {
	l, err := NewHPClientLimiter(HPLimiterOpts{
		Threshold: 4,
	})
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		addr := netip.AddrFrom4([4]byte{(byte)(i)})
		l.AcquireToken(addr)
	}
}
