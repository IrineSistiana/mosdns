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

package domain

import (
	"strings"
)

type ReverseDomainScanner struct {
	s string // not fqdn
	p int
	t int
}

func NewReverseDomainScanner(s string) *ReverseDomainScanner {
	s = TrimDot(s)
	return &ReverseDomainScanner{
		s: s,
		p: len(s),
		t: len(s),
	}
}

func (s *ReverseDomainScanner) Scan() bool {
	if s.p <= 0 {
		return false
	}
	s.t = s.p
	s.p = strings.LastIndexByte(s.s[:s.p], '.')
	return true
}

func (s *ReverseDomainScanner) NextLabelOffset() int {
	return s.p + 1
}

func (s *ReverseDomainScanner) NextLabel() (label string) {
	return s.s[s.p+1 : s.t]
}

// NormalizeDomain normalize domain string s.
// It removes the suffix "." and make sure the domain is in lower case.
// e.g. a fqdn "GOOGLE.com." will become "google.com"
func NormalizeDomain(s string) string {
	return strings.ToLower(TrimDot(s))
}

// TrimDot trims suffix '.'
func TrimDot(s string) string {
	if len(s) >= 1 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

// labelNode can store dns labels.
type labelNode[T any] struct {
	children map[string]*labelNode[T] // lazy init

	v    T
	hasV bool
}

func (n *labelNode[T]) storeValue(v T) {
	n.v = v
	n.hasV = true
}

func (n *labelNode[T]) getValue() (T, bool) {
	return n.v, n.hasV
}

func (n *labelNode[T]) hasValue() bool {
	return n.hasV
}

func (n *labelNode[T]) newChild(key string) *labelNode[T] {
	if n.children == nil {
		n.children = make(map[string]*labelNode[T])
	}
	node := new(labelNode[T])
	n.children[key] = node
	return node
}

func (n *labelNode[T]) getChild(key string) *labelNode[T] {
	return n.children[key]
}

func (n *labelNode[T]) len() int {
	l := 0
	for _, node := range n.children {
		l += node.len()
		if node.hasValue() {
			l++
		}
	}
	return l
}
