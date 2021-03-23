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
	"github.com/miekg/dns"
	"strings"
)

type DomainMatcher struct {
	root *LabelNode
}

func NewDomainMatcher() *DomainMatcher {
	return &DomainMatcher{root: new(LabelNode)}
}

func SplitLatestLabel(s string) (label string, remain string) {
	s = trimDot(s)
	l := strings.LastIndexByte(s, '.')
	if l == -1 {
		return s, ""
	}
	return s[l+1:], s[:l]
}

func (m *DomainMatcher) Match(fqdn string) (interface{}, bool) {
	currentNode := m.root
	remain := fqdn
	var label string
	for {
		if currentNode.IsEnd() {
			return currentNode.GetValue(), true
		}
		if len(remain) == 0 {
			return nil, false // end of domain
		}
		label, remain = SplitLatestLabel(remain)
		if currentNode = currentNode.GetChild(label); currentNode == nil {
			return nil, false // end of tree
		}
	}
}

func (m *DomainMatcher) Len() int {
	return m.root.Len()
}

func (m *DomainMatcher) Add(domain string, v interface{}) error {
	m.add(dns.Fqdn(domain), v)
	return nil
}

func (m *DomainMatcher) add(fqdn string, v interface{}) {
	currentNode := m.root
	remain := fqdn
	var label string
	for {
		if len(remain) == 0 || remain == "." {
			break // end of domain
		}
		if currentNode.IsEnd() {
			return // end of tree. This domain is redundant.
		}

		label, remain = SplitLatestLabel(remain)
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
	children *childMap // lazy init

	isEnd bool
	v     interface{}
}

type childMap struct {
	s map[[8]byte]*LabelNode  // lazy init
	l map[[16]byte]*LabelNode // lazy init
	x map[string]*LabelNode   // lazy init
}

func (cm *childMap) len() int {
	sum := 0
	for _, node := range cm.s {
		if node.IsEnd() {
			sum++
		} else {
			sum += node.Len()
		}
	}
	for _, node := range cm.l {
		if node.IsEnd() {
			sum++
		} else {
			sum += node.Len()
		}
	}
	for _, node := range cm.x {
		if node.IsEnd() {
			sum++
		} else {
			sum += node.Len()
		}
	}
	return sum
}

func (cm *childMap) get(key string) *LabelNode {
	l := len(key)
	switch {
	case l <= 8:
		m := cm.s
		var b [8]byte
		copy(b[:], key)
		return m[b]
	case l <= 16:
		m := cm.l
		var b [16]byte
		copy(b[:], key)
		return m[b]
	default:
		m := cm.x
		return m[key]
	}
}

func (cm *childMap) store(key string, node *LabelNode) {
	l := len(key)
	switch {
	case l <= 8:
		if cm.s == nil {
			cm.s = make(map[[8]byte]*LabelNode)
		}
		m := cm.s
		var b [8]byte
		copy(b[:], key)
		m[b] = node
	case l <= 16:
		if cm.l == nil {
			cm.l = make(map[[16]byte]*LabelNode)
		}
		m := cm.l
		var b [16]byte
		copy(b[:], key)
		m[b] = node
	default:
		if cm.x == nil {
			cm.x = make(map[string]*LabelNode)
		}
		m := cm.x
		m[key] = node
	}
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
		n.children = new(childMap)
	}
	node := new(LabelNode)
	n.children.store(key, node)
	return node
}

func (n *LabelNode) GetChild(key string) *LabelNode {
	if n.children == nil {
		return nil
	}
	return n.children.get(key)
}

func (n *LabelNode) Len() int {
	if n.children == nil {
		return 0
	}
	return n.children.len()
}
