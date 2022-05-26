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

package domain

import (
	"strings"
)

var _ Matcher[any] = (*DomainMatcher[any])(nil)

type DomainMatcher[T any] struct {
	root *LabelNode[T]
}

func NewDomainMatcher[T any]() *DomainMatcher[T] {
	return &DomainMatcher[T]{root: new(LabelNode[T])}
}

func (m *DomainMatcher[T]) Match(s string) (T, bool) {
	currentNode := m.root
	ds := NewUnifiedDomainScanner(s)
	var v T
	var ok bool
	for ds.Scan() {
		label, _ := ds.PrevLabel()
		if nextNode := currentNode.GetChild(label); nextNode != nil {
			if nextNode.HasValue() {
				v, ok = nextNode.GetValue()
			}
			currentNode = nextNode
		} else {
			break
		}
	}
	return v, ok
}

func (m *DomainMatcher[T]) Len() int {
	return m.root.Len()
}

func (m *DomainMatcher[T]) Add(s string, v T) error {
	currentNode := m.root
	ds := NewUnifiedDomainScanner(s)
	for ds.Scan() {
		label, _ := ds.PrevLabel()
		if child := currentNode.GetChild(label); child != nil {
			currentNode = child
		} else {
			currentNode = currentNode.NewChild(label)
		}
	}
	currentNode.StoreValue(v)
	return nil
}

// LabelNode can store dns labels.
type LabelNode[T any] struct {
	children map[string]*LabelNode[T] // lazy init

	v    T
	hasV bool
}

func (n *LabelNode[T]) StoreValue(v T) {
	n.v = v
	n.hasV = true
}

func (n *LabelNode[T]) GetValue() (T, bool) {
	return n.v, n.hasV
}

func (n *LabelNode[T]) HasValue() bool {
	return n.hasV
}

func (n *LabelNode[T]) NewChild(key string) *LabelNode[T] {
	if n.children == nil {
		n.children = make(map[string]*LabelNode[T])
	}
	node := new(LabelNode[T])
	n.children[key] = node
	return node
}

func (n *LabelNode[T]) GetChild(key string) *LabelNode[T] {
	return n.children[key]
}

func (n *LabelNode[T]) Len() int {
	l := 0
	for _, node := range n.children {
		l += node.Len()
		if node.HasValue() {
			l++
		}
	}
	return l
}

type DomainScanner struct {
	d string
	n int
}

func NewUnifiedDomainScanner(s string) *DomainScanner {
	domain := UnifyDomain(s)
	return &DomainScanner{
		d: domain,
		n: len(domain),
	}
}

func (s *DomainScanner) Scan() bool {
	return s.n > 0
}

func (s *DomainScanner) PrevLabelOffset() int {
	s.n = strings.LastIndexByte(s.d[:s.n], '.')
	return s.n + 1
}

func (s *DomainScanner) PrevLabel() (label string, end bool) {
	n := strings.LastIndexByte(s.d[:s.n], '.')
	l := s.d[n+1 : s.n]
	s.n = n
	return l, n == -1
}

func (s *DomainScanner) PrevSubDomain() (sub string, end bool) {
	n := strings.LastIndexByte(s.d[:s.n], '.')
	s.n = n
	return s.d[n+1:], n == -1
}
