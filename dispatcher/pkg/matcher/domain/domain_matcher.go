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

type DomainMatcher struct {
	root *LabelNode
}

func NewDomainMatcher() *DomainMatcher {
	return &DomainMatcher{root: new(LabelNode)}
}

func (m *DomainMatcher) Match(s string) (interface{}, bool) {
	currentNode := m.root
	ds := NewDomainScanner(s)
	for ds.Scan() {
		label := ds.PrevLabel()
		if currentNode = currentNode.GetChild(label); currentNode == nil {
			return nil, false // end of tree, not matched
		}
		if currentNode.IsEnd() {
			return currentNode.GetValue(), true // end node, matched
		}
	}
	return nil, false // end of domain (short domain), not matched.
}

func (m *DomainMatcher) Len() int {
	return m.root.Len()
}

func (m *DomainMatcher) Add(s string, v interface{}) error {
	m.add(s, v)
	return nil
}

func (m *DomainMatcher) add(s string, v interface{}) {
	// find the end node
	currentNode := m.root
	ds := NewDomainScanner(s)
	for ds.Scan() {
		if currentNode.IsEnd() { // reach a end node, the new domain is redundant.
			return
		}
		label := ds.PrevLabel()
		if child := currentNode.GetChild(label); child != nil {
			currentNode = child
		} else {
			currentNode = currentNode.NewChild(label)
		}
	}

	currentNode.MarkAsEndNode()
	oldV := currentNode.GetValue()
	if appendAble, ok := oldV.(Appendable); ok {
		appendAble.Append(v)
	} else {
		currentNode.StoreValue(v) // overwrite
	}
}

// LabelNode can store dns labels efficiently.
type LabelNode struct {
	children map[string]*LabelNode // lazy init

	isEnd bool
	v     interface{}
}

func (n *LabelNode) StoreValue(v interface{}) {
	n.v = v
}

func (n *LabelNode) GetValue() interface{} {
	return n.v
}

func (n *LabelNode) MarkAsEndNode() {
	// remove all its children
	n.children = nil
	n.isEnd = true
}

func (n *LabelNode) IsEnd() bool {
	return n.isEnd
}

func (n *LabelNode) NewChild(key string) *LabelNode {
	if n.children == nil {
		n.children = make(map[string]*LabelNode)
	}
	node := new(LabelNode)
	n.children[key] = node
	return node
}

func (n *LabelNode) GetChild(key string) *LabelNode {
	return n.children[key]
}

func (n *LabelNode) Len() int {
	l := 0
	for _, node := range n.children {
		l += node.Len()
		if node.IsEnd() {
			l++
		}
	}
	return l
}

// SimpleDomainMatcher just like DomainMatcher, but it can not
// store value.
// It allocates less memory than DomainMatcher.
type SimpleDomainMatcher struct {
	s map[[16]byte]bool
	m map[[32]byte]bool
	l map[string]bool
}

func NewSimpleDomainMatcher() *SimpleDomainMatcher {
	return &SimpleDomainMatcher{
		s: make(map[[16]byte]bool),
		m: make(map[[32]byte]bool),
		l: make(map[string]bool),
	}
}

func (m *SimpleDomainMatcher) Add(s string, _ interface{}) error {
	domain := trimDot(s)
	ds := NewDomainScanner(domain)
	for ds.Scan() {
		off := ds.PrevLabelOffset()
		sub := domain[off:]
		isEnd := off == 0
		subEnd, ok := m.fullMatch(sub)
		if subEnd {
			return nil // redundant domain
		}
		if !ok || subEnd != isEnd {
			m.add(sub, isEnd)
		}
	}
	return nil
}

func (m *SimpleDomainMatcher) add(domain string, isEnd bool) {
	n := len(domain)
	switch {
	case n <= 16:
		var key [16]byte
		copy(key[:], domain)
		m.s[key] = isEnd
	case n <= 32:
		var key [32]byte
		copy(key[:], domain)
		m.m[key] = isEnd
	default:
		m.l[domain] = isEnd
	}
}

func (m *SimpleDomainMatcher) Match(s string) (v interface{}, ok bool) {
	ok = m.match(s)
	return nil, ok
}

type DomainScanner struct {
	d string
	n int
}

func (m *SimpleDomainMatcher) match(s string) bool {
	domain := trimDot(s)
	ds := NewDomainScanner(domain)
	for ds.Scan() {
		off := ds.PrevLabelOffset()
		isEnd, ok := m.fullMatch(domain[off:])
		if !ok { // no such sub domain
			return false
		}
		if isEnd {
			return true
		}
	}
	return false
}

func (m *SimpleDomainMatcher) fullMatch(domain string) (isEnd, ok bool) {
	n := len(domain)
	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], domain)
		isEnd, ok = m.s[b]
		return
	case n <= 32:
		var b [32]byte
		copy(b[:], domain)
		isEnd, ok = m.m[b]
		return
	default:
		isEnd, ok = m.l[domain]
		return
	}
}

func (m *SimpleDomainMatcher) Len() int {
	return len(m.l) + len(m.m) + len(m.s)
}

func NewDomainScanner(s string) *DomainScanner {
	domain := trimDot(s)
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

func (s *DomainScanner) PrevLabel() string {
	n := strings.LastIndexByte(s.d[:s.n], '.')
	l := s.d[n+1 : s.n]
	s.n = n
	return l
}

func (s *DomainScanner) PrevSubDomain() string {
	n := strings.LastIndexByte(s.d[:s.n], '.')
	l := s.d[n+1:]
	s.n = n
	return l
}
