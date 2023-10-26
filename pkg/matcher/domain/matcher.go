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
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
)

var _ WriteableMatcher[any] = (*MixMatcher[any])(nil)
var _ WriteableMatcher[any] = (*SubDomainMatcher[any])(nil)
var _ WriteableMatcher[any] = (*FullMatcher[any])(nil)
var _ WriteableMatcher[any] = (*KeywordMatcher[any])(nil)
var _ WriteableMatcher[any] = (*RegexMatcher[any])(nil)

type SubDomainMatcher[T any] struct {
	root *labelNode[T]
}

func NewSubDomainMatcher[T any]() *SubDomainMatcher[T] {
	return &SubDomainMatcher[T]{root: new(labelNode[T])}
}

func (m *SubDomainMatcher[T]) Match(s string) (T, bool) {
	s = NormalizeDomain(s)
	ds := NewReverseDomainScanner(s)
	currentNode := m.root
	v, ok := currentNode.getValue()
	for ds.Scan() {
		label := ds.NextLabel()
		if nextNode := currentNode.getChild(label); nextNode != nil {
			if nextNode.hasValue() {
				v, ok = nextNode.getValue()
			}
			currentNode = nextNode
		} else {
			break
		}
	}
	return v, ok
}

func (m *SubDomainMatcher[T]) Len() int {
	return m.root.len()
}

func (m *SubDomainMatcher[T]) Add(s string, v T) error {
	s = NormalizeDomain(s)
	ds := NewReverseDomainScanner(s)
	currentNode := m.root
	for ds.Scan() {
		label := ds.NextLabel()
		if child := currentNode.getChild(label); child != nil {
			currentNode = child
		} else {
			currentNode = currentNode.newChild(label)
		}
	}
	currentNode.storeValue(v)
	return nil
}

type FullMatcher[T any] struct {
	m map[string]T // string in is map must be a normalized domain (See NormalizeDomain).
}

func NewFullMatcher[T any]() *FullMatcher[T] {
	return &FullMatcher[T]{
		m: make(map[string]T),
	}
}

// Add adds domain s to this matcher, s can be a fqdn or not.
func (m *FullMatcher[T]) Add(s string, v T) error {
	s = NormalizeDomain(s)
	m.m[s] = v
	return nil
}

func (m *FullMatcher[T]) Match(s string) (v T, ok bool) {
	s = NormalizeDomain(s)
	v, ok = m.m[s]
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
	keyword = NormalizeDomain(keyword) // fqdn-insensitive and case-insensitive
	m.kws[keyword] = v
	return nil
}

func (m *KeywordMatcher[T]) Match(s string) (v T, ok bool) {
	s = NormalizeDomain(s)
	for k, v := range m.kws {
		if strings.Contains(s, k) {
			return v, true
		}
	}
	return v, false
}

func (m *KeywordMatcher[T]) Len() int {
	return len(m.kws)
}

// RegexMatcher contains regexp rules.
// Note: the regexp rule is expect to match a lower-case non fqdn.
type RegexMatcher[T any] struct {
	regs map[string]*regElem[T]
}

type regElem[T any] struct {
	reg *regexp.Regexp
	v   T
}

func NewRegexMatcher[T any]() *RegexMatcher[T] {
	return &RegexMatcher[T]{regs: make(map[string]*regElem[T])}
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
	s = NormalizeDomain(s)
	for _, e := range m.regs {
		if e.reg.MatchString(s) {
			return e.v, true
		}
	}
	var zeroT T
	return zeroT, false
}

func (m *RegexMatcher[T]) Len() int {
	return len(m.regs)
}

const (
	MatcherFull    = "full"
	MatcherDomain  = "domain"
	MatcherRegexp  = "regexp"
	MatcherKeyword = "keyword"
)

type MixMatcher[T any] struct {
	defaultMatcher string

	full    *FullMatcher[T]
	domain  *SubDomainMatcher[T]
	regex   *RegexMatcher[T]
	keyword *KeywordMatcher[T]
}

func NewMixMatcher[T any]() *MixMatcher[T] {
	return &MixMatcher[T]{
		full:    NewFullMatcher[T](),
		domain:  NewSubDomainMatcher[T](),
		regex:   NewRegexMatcher[T](),
		keyword: NewKeywordMatcher[T](),
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

var ErrNodefaultMatcher = errors.New("default matcher is not set")

func (m *MixMatcher[T]) Add(s string, v T) error {
	typ, pattern := m.splitTypeAndPattern(s)
	if len(typ) == 0 {
		if len(m.defaultMatcher) != 0 {
			typ = m.defaultMatcher
		} else {
			return ErrNodefaultMatcher
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
	for _, matcher := range [...]interface{ Len() int }{m.full, m.domain, m.regex, m.keyword} {
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
