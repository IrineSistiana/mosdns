//     Copyright (C) 2020, IrineSistiana
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

package domain

import (
	"github.com/miekg/dns"
)

type ListMatcher struct {
	s map[[16]byte]struct{}
	m map[[32]byte]struct{}
	l map[[256]byte]struct{}
}

func NewListMatcher() *ListMatcher {
	return &ListMatcher{
		s: make(map[[16]byte]struct{}),
		m: make(map[[32]byte]struct{}),
		l: make(map[[256]byte]struct{}),
	}
}

func (l *ListMatcher) Add(fqdn string) {
	n := len(fqdn)

	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], fqdn)
		l.s[b] = struct{}{}
	case n <= 32:
		var b [32]byte
		copy(b[:], fqdn)
		l.m[b] = struct{}{}
	default:
		var b [256]byte
		copy(b[:], fqdn)
		l.l[b] = struct{}{}
	}
}

func (l *ListMatcher) Match(fqdn string) bool {
	if fqdn == "." {
		return false
	}
	idx := make([]int, 1, 6)
	off := 0
	end := false

	for {
		off, end = dns.NextLabel(fqdn, off)
		if end {
			break
		}
		idx = append(idx, off)
	}

	for i := range idx {
		p := idx[len(idx)-1-i]
		if l.has(fqdn[p:]) {
			return true
		}
	}
	return false
}

func (l *ListMatcher) has(fqdn string) bool {
	n := len(fqdn)
	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], fqdn)
		_, ok := l.s[b]
		return ok
	case n <= 32:
		var b [32]byte
		copy(b[:], fqdn)
		_, ok := l.m[b]
		return ok
	default:
		var b [256]byte
		copy(b[:], fqdn)
		_, ok := l.l[b]
		return ok
	}
}

func (l *ListMatcher) Len() int {
	return len(l.l) + len(l.m) + len(l.s)
}
