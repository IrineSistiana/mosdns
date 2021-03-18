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

func PrevLabel(s string) (string, int) {
	if len(s) == 0 {
		return "", -1
	}
	if s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}

	l := strings.LastIndexByte(s, '.') + 1
	return s[l:], l
}

func (m *DomainMatcher) Match(fqdn string) (interface{}, bool) {
	currentNode := m.root
	offset := len(fqdn)
	var label string
	for {
		if currentNode.IsEnd() {
			return currentNode.GetValue(), true
		}

		label, offset = PrevLabel(fqdn[:offset])
		if offset == -1 {
			return nil, false // end of domain
		}
		if currentNode = currentNode.GetChild(label); currentNode == nil {
			return nil, false // end of tree
		}
	}
}

func (m *DomainMatcher) Len() int {
	return m.root.Len()
}

func (m *DomainMatcher) Add(domain string, v interface{}) error {
	m.add(domain, v)
	return nil
}

func (m *DomainMatcher) add(domain string, v interface{}) {
	currentNode := m.root
	offset := len(domain)
	var label string
	for {
		label, offset = PrevLabel(domain[:offset])
		if offset == -1 {
			break // end of domain
		}

		if currentNode.IsEnd() {
			return // end of tree. This domain is redundant.
		}

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
