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
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"regexp"
	"strings"
	"sync"
)

var _ WriteableMatcher[any] = (*MixMatcher[any])(nil)
var _ WriteableMatcher[any] = (*SubDomainMatcher[any])(nil)
var _ WriteableMatcher[any] = (*FullMatcher[any])(nil)
var _ WriteableMatcher[any] = (*KeywordMatcher[any])(nil)
var _ WriteableMatcher[any] = (*RegexMatcher[any])(nil)

type SubDomainMatcher[T any] struct {
	root *LabelNode[T]
}

func NewSubDomainMatcher[T any]() *SubDomainMatcher[T] {
	return &SubDomainMatcher[T]{root: new(LabelNode[T])}
}

func (m *SubDomainMatcher[T]) Match(s string) (T, bool) {
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

func (m *SubDomainMatcher[T]) Len() int {
	return m.root.Len()
}

func (m *SubDomainMatcher[T]) Add(s string, v T) error {
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

type FullMatcher[T any] struct {
	m map[string]T // string must be a fqdn.
}

func NewFullMatcher[T any]() *FullMatcher[T] {
	return &FullMatcher[T]{
		m: make(map[string]T),
	}
}

func (m *FullMatcher[T]) Add(s string, v T) error {
	m.m[UnifyDomain(s)] = v
	return nil
}

func (m *FullMatcher[T]) Match(s string) (v T, ok bool) {
	v, ok = m.m[UnifyDomain(s)]
	return
}

func (m *FullMatcher[T]) Len() int {
	return len(m.m)
}

type KeywordMatcher[T any] struct {
	kws map[string]T
}

func NewKeywordMatcher[T any]() *KeywordMatcher[T] {
	return &KeywordMatcher[T]{
		kws: make(map[string]T),
	}
}

func (m *KeywordMatcher[T]) Add(keyword string, v T) error {
	m.kws[keyword] = v
	return nil
}

func (m *KeywordMatcher[T]) Match(s string) (v T, ok bool) {
	domain := UnifyDomain(s)
	for k, v := range m.kws {
		if strings.Contains(domain, k) {
			return v, true
		}
	}
	return v, false
}

func (m *KeywordMatcher[T]) Len() int {
	return len(m.kws)
}

type RegexMatcher[T any] struct {
	regs  map[string]*regElem[T]
	cache *regCache[T]
}

type regElem[T any] struct {
	reg *regexp.Regexp
	v   T
}

func NewRegexMatcher[T any]() *RegexMatcher[T] {
	return &RegexMatcher[T]{regs: make(map[string]*regElem[T])}
}

func NewRegexMatcherWithCache[T any](cap int) *RegexMatcher[T] {
	return &RegexMatcher[T]{regs: make(map[string]*regElem[T]), cache: newRegCache[T](cap)}
}

func (m *RegexMatcher[T]) Add(expr string, v T) error {
	e := m.regs[expr]
	if e == nil {
		reg, err := regexp.Compile(expr)
		if err != nil {
			return err
		}
		m.regs[expr] = &regElem[T]{
			reg: reg,
			v:   v,
		}
	} else {
		e.v = v
	}

	return nil
}

func (m *RegexMatcher[T]) Match(s string) (v T, ok bool) {
	return m.match(TrimDot(s))
}

func (m *RegexMatcher[T]) match(domain string) (v T, ok bool) {
	if m.cache != nil {
		if e, ok := m.cache.lookup(domain); ok { // cache hit
			if e != nil {
				return e.v, true // matched
			}
			var zeroT T
			return zeroT, false // not matched
		}
	}

	for _, e := range m.regs {
		if e.reg.MatchString(domain) {
			if m.cache != nil {
				m.cache.cache(domain, e)
			}
			return e.v, true
		}
	}

	if m.cache != nil { // cache the string
		m.cache.cache(domain, nil)
	}
	var zeroT T
	return zeroT, false
}

func (m *RegexMatcher[T]) Len() int {
	return len(m.regs)
}

func (m *RegexMatcher[T]) ResetCache() {
	if m.cache != nil {
		m.cache.reset()
	}
}

type regCache[T any] struct {
	cap int
	sync.RWMutex
	m map[string]*regElem[T]
}

func newRegCache[T any](cap int) *regCache[T] {
	return &regCache[T]{
		cap: cap,
		m:   make(map[string]*regElem[T], cap),
	}
}

func (c *regCache[T]) cache(s string, res *regElem[T]) {
	c.Lock()
	defer c.Unlock()

	c.tryEvictUnderLock()
	c.m[s] = res
}

func (c *regCache[T]) lookup(s string) (res *regElem[T], ok bool) {
	c.RLock()
	defer c.RUnlock()
	res, ok = c.m[s]
	return
}

func (c *regCache[T]) reset() {
	c.Lock()
	defer c.Unlock()
	c.m = make(map[string]*regElem[T], c.cap)
}

func (c *regCache[T]) tryEvictUnderLock() {
	if len(c.m) >= c.cap {
		i := c.cap / 8
		for key := range c.m { // evict 1/8 cache
			delete(c.m, key)
			i--
			if i < 0 {
				return
			}
		}
	}
}

const (
	MatcherFull    = "full"
	MatcherDomain  = "domain"
	MatcherRegexp  = "regexp"
	MatcherKeyword = "keyword"
)

type MixMatcher[T any] struct {
	defaultMatcher string

	full    WriteableMatcher[T]
	domain  WriteableMatcher[T]
	regex   WriteableMatcher[T]
	keyword WriteableMatcher[T]
}

func NewMixMatcher[T any]() *MixMatcher[T] {
	return &MixMatcher[T]{
		defaultMatcher: MatcherFull,
		full:           NewFullMatcher[T](),
		domain:         NewSubDomainMatcher[T](),
		regex:          NewRegexMatcher[T](),
		keyword:        NewKeywordMatcher[T](),
	}
}

func (m *MixMatcher[T]) SetDefaultMatcher(s string) {
	m.defaultMatcher = s
}

func (m *MixMatcher[T]) GetSubMatcher(typ string) WriteableMatcher[T] {
	switch typ {
	case MatcherFull:
		return m.full
	case MatcherDomain:
		return m.domain
	case MatcherRegexp:
		return m.regex
	case MatcherKeyword:
		return m.keyword
	}
	return nil
}

func (m *MixMatcher[T]) Add(s string, v T) error {
	typ, pattern := m.splitTypeAndPattern(s)
	if len(typ) == 0 {
		if len(m.defaultMatcher) != 0 {
			typ = m.defaultMatcher
		} else {
			typ = MatcherFull
		}
	}
	sm := m.GetSubMatcher(typ)
	if sm == nil {
		return fmt.Errorf("unsupported match type [%s]", typ)
	}
	return sm.Add(pattern, v)
}

func (m *MixMatcher[T]) Match(s string) (v T, ok bool) {
	for _, matcher := range [...]Matcher[T]{m.full, m.domain, m.regex, m.keyword} {
		if v, ok = matcher.Match(s); ok {
			return v, true
		}
	}
	return
}

func (m *MixMatcher[T]) Len() int {
	sum := 0
	for _, matcher := range [...]Matcher[T]{m.full, m.domain, m.regex, m.keyword} {
		if matcher == nil {
			continue
		}
		sum += matcher.Len()
	}
	return sum
}

func (m *MixMatcher[T]) splitTypeAndPattern(s string) (string, string) {
	typ, pattern, ok := utils.SplitString2(s, ":")
	if !ok {
		pattern = s
	}
	return typ, pattern
}

// TrimDot trims the suffix '.'.
func TrimDot(s string) string {
	return strings.TrimSuffix(s, ".")
}

// UnifyDomain unifies domain strings.
// It removes the suffix "." and make sure the domain is in lower case.
func UnifyDomain(s string) string {
	return strings.ToLower(TrimDot(s))
}
