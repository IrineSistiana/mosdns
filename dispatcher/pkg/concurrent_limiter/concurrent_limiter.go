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

import "fmt"

// ConcurrentLimiter is a soft limiter.
type ConcurrentLimiter struct {
	bucket chan struct{}
}

// NewConcurrentLimiter returns a ConcurrentLimiter, max must > 0.
func NewConcurrentLimiter(max int) *ConcurrentLimiter {
	if max <= 0 {
		panic(fmt.Sprintf("ConcurrentLimiter: invalid max arg: %d", max))
	}

	bucket := make(chan struct{}, max)
	for i := 0; i < max; i++ {
		bucket <- struct{}{}
	}

	return &ConcurrentLimiter{bucket: bucket}
}

func (l *ConcurrentLimiter) Wait() <-chan struct{} {
	return l.bucket
}

func (l *ConcurrentLimiter) Done() {
	select {
	case l.bucket <- struct{}{}:
	default:
		panic("ConcurrentLimiter: bucket overflow")
	}
}

func (l *ConcurrentLimiter) Available() int {
	return len(l.bucket)
}
