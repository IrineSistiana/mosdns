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

type DomainMatcher struct {
	s map[[16]byte]interface{}
	m map[[32]byte]interface{}
	l map[[256]byte]interface{}
}

func NewListMatcher() *DomainMatcher {
	return &DomainMatcher{
		s: make(map[[16]byte]interface{}),
		m: make(map[[32]byte]interface{}),
		l: make(map[[256]byte]interface{}),
	}
}

func (l *DomainMatcher) Add(fqdn string, v interface{}) {
	n := len(fqdn)

	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], fqdn)
		l.s[b] = v
	case n <= 32:
		var b [32]byte
		copy(b[:], fqdn)
		l.m[b] = v
	default:
		var b [256]byte
		copy(b[:], fqdn)
		l.l[b] = v
	}
}

func (l *DomainMatcher) Match(fqdn string) (v interface{}, ok bool) {
	if fqdn == "." {
		return nil, false
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
		if v, ok = l.fullMatch(fqdn[p:]); ok {
			return v, true
		}
	}
	return nil, false
}

func (l *DomainMatcher) fullMatch(fqdn string) (v interface{}, ok bool) {
	n := len(fqdn)
	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], fqdn)
		v, ok = l.s[b]
		return
	case n <= 32:
		var b [32]byte
		copy(b[:], fqdn)
		v, ok = l.m[b]
		return
	default:
		var b [256]byte
		copy(b[:], fqdn)
		v, ok = l.l[b]
		return
	}
}

func (l *DomainMatcher) Len() int {
	return len(l.l) + len(l.m) + len(l.s)
}
