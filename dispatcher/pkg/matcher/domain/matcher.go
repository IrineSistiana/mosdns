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
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"regexp"
	"strings"
)

type DomainMatcher struct {
	mode DomainMatcherMode

	s map[[16]byte]interface{}
	m map[[32]byte]interface{}
	l map[[256]byte]interface{}
}

type DomainMatcherMode uint8

const (
	DomainMatcherModeDomain DomainMatcherMode = iota
	DomainMatcherModeFull
)

func NewDomainMatcher(mode DomainMatcherMode) *DomainMatcher {
	return &DomainMatcher{
		mode: mode,
		s:    make(map[[16]byte]interface{}),
		m:    make(map[[32]byte]interface{}),
		l:    make(map[[256]byte]interface{}),
	}
}

func (m *DomainMatcher) Add(domain string, v interface{}) error {
	m.add(domain, v)
	return nil
}

func (m *DomainMatcher) add(domain string, v interface{}) {
	fqdn := dns.Fqdn(domain)
	n := len(fqdn)

	var old interface{}
	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], fqdn)
		mm := m.s
		if old = mm[b]; old == nil {
			mm[b] = v
		}
	case n <= 32:
		var b [32]byte
		copy(b[:], fqdn)
		mm := m.m
		if old = mm[b]; old == nil {
			mm[b] = v
		}
	default:
		var b [256]byte
		copy(b[:], fqdn)
		mm := m.l
		if old = mm[b]; old == nil {
			mm[b] = v
		}
	}

	if old != nil && v != nil {
		if appendable, ok := old.(Appendable); ok {
			appendable.Append(v)
		}
	}
}

func (m *DomainMatcher) Del(domain string) {
	fqdn := dns.Fqdn(domain)
	n := len(fqdn)
	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], fqdn)
		mm := m.s
		delete(mm, b)
	case n <= 32:
		var b [32]byte
		copy(b[:], fqdn)
		mm := m.m
		delete(mm, b)
	default:
		var b [256]byte
		copy(b[:], fqdn)
		mm := m.l
		delete(mm, b)
	}
}

func (m *DomainMatcher) Match(fqdn string) (v interface{}, ok bool) {
	switch m.mode {
	case DomainMatcherModeFull:
		return m.fullMatch(fqdn)
	case DomainMatcherModeDomain:
		return m.domainMatch(fqdn)
	default:
		panic(fmt.Sprintf("domain: invalid match mode %d", m.mode))
	}
}

func (m *DomainMatcher) domainMatch(fqdn string) (v interface{}, ok bool) {
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
		if v, ok = m.fullMatch(fqdn[p:]); ok {
			return v, true
		}
	}
	return nil, false
}

func (m *DomainMatcher) fullMatch(fqdn string) (v interface{}, ok bool) {
	n := len(fqdn)
	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], fqdn)
		v, ok = m.s[b]
		return
	case n <= 32:
		var b [32]byte
		copy(b[:], fqdn)
		v, ok = m.m[b]
		return
	default:
		var b [256]byte
		copy(b[:], fqdn)
		v, ok = m.l[b]
		return
	}
}

func (m *DomainMatcher) Len() int {
	return len(m.l) + len(m.m) + len(m.s)
}

type KeywordMatcher struct {
	kws map[string]interface{}
}

func NewKeywordMatcher() *KeywordMatcher {
	return &KeywordMatcher{
		kws: make(map[string]interface{}),
	}
}

func (m *KeywordMatcher) Add(keyword string, v interface{}) error {
	m.add(keyword, v)
	return nil
}

func (m *KeywordMatcher) add(keyword string, v interface{}) {
	o := m.kws[keyword]
	if o == nil {
		m.kws[keyword] = v
	} else if v != nil {
		if appendable, ok := o.(Appendable); ok {
			appendable.Append(v)
		}
	}
}

func (m *KeywordMatcher) Del(keyword string) {
	delete(m.kws, keyword)
}

func (m *KeywordMatcher) Match(fqdn string) (v interface{}, ok bool) {
	for k, v := range m.kws {
		if strings.Contains(fqdn, k) {
			return v, true
		}
	}
	return nil, false
}

func (m *KeywordMatcher) Len() int {
	return len(m.kws)
}

type RegexMatcher struct {
	regs map[string]*regElem
}

type regElem struct {
	reg *regexp.Regexp
	v   interface{}
}

func NewRegexMatcher() *RegexMatcher {
	return &RegexMatcher{regs: make(map[string]*regElem)}
}

func (m *RegexMatcher) Add(expr string, v interface{}) error {
	e := m.regs[expr]
	if e == nil {
		reg, err := regexp.Compile(expr)
		if err != nil {
			return err
		}
		m.regs[expr] = &regElem{
			reg: reg,
			v:   v,
		}
	} else if v != nil {
		if e.v == nil {
			e.v = v
		} else if appendable, ok := e.v.(Appendable); ok {
			appendable.Append(v)
		}
	}

	return nil
}

func (m *RegexMatcher) Del(expr string) {
	delete(m.regs, expr)
}

func (m *RegexMatcher) Match(fqdn string) (v interface{}, ok bool) {
	for _, e := range m.regs {
		if e.reg.MatchString(fqdn) {
			return e.v, true
		}
	}
	return nil, false
}

func (m *RegexMatcher) Len() int {
	return len(m.regs)
}

type MixMatcherPatternType uint8

const (
	MixMatcherPatternTypeDomain MixMatcherPatternType = iota
	MixMatcherPatternTypeFull
	MixMatcherPatternTypeKeyword
	MixMatcherPatternTypeRegexp
)

type MixMatcher struct {
	typMap map[string]MixMatcherPatternType

	keyword *KeywordMatcher
	regex   *RegexMatcher
	domain  *DomainMatcher
	full    *DomainMatcher
}

func NewMixMatcher() *MixMatcher {
	return &MixMatcher{
		keyword: NewKeywordMatcher(),
		regex:   NewRegexMatcher(),
		domain:  NewDomainMatcher(DomainMatcherModeDomain),
		full:    NewDomainMatcher(DomainMatcherModeFull),
	}
}

var defaultStrToPatternType = map[string]MixMatcherPatternType{
	"":        MixMatcherPatternTypeDomain,
	"domain":  MixMatcherPatternTypeDomain,
	"keyword": MixMatcherPatternTypeKeyword,
	"regexp":  MixMatcherPatternTypeRegexp,
	"full":    MixMatcherPatternTypeFull,
}

func (m *MixMatcher) SetPattenTypeMap(typMap map[string]MixMatcherPatternType) {
	m.typMap = typMap
}

func (m *MixMatcher) Add(pattern string, v interface{}) error {
	typ, pattern, err := m.splitTypeAndPattern(pattern)
	if err != nil {
		return err
	}
	return m.AddElem(typ, pattern, v)
}

func (m *MixMatcher) AddElem(typ MixMatcherPatternType, pattern string, v interface{}) error {
	return m.getSubMatcher(typ).Add(pattern, v)
}

func (m *MixMatcher) Del(pattern string) {
	typ, pattern, err := m.splitTypeAndPattern(pattern)
	if err != nil {
		return
	}
	m.getSubMatcher(typ).Del(pattern)
}

func (m *MixMatcher) Match(fqdn string) (v interface{}, ok bool) {
	// it seems v2ray match full matcher first, then domain, reg and keyword matcher.
	if v, ok = m.full.Match(fqdn); ok {
		return
	}
	if v, ok = m.domain.Match(fqdn); ok {
		return
	}
	if v, ok = m.regex.Match(fqdn); ok {
		return
	}
	if v, ok = m.keyword.Match(fqdn); ok {
		return
	}
	return
}

func (m *MixMatcher) Len() int {
	sum := 0
	for _, m := range [...]Matcher{m.domain, m.keyword, m.regex, m.full} {
		sum += m.Len()
	}
	return sum
}

func (m *MixMatcher) splitTypeAndPattern(pattern string) (MixMatcherPatternType, string, error) {
	typMap := m.typMap
	if typMap == nil {
		typMap = defaultStrToPatternType
	}

	typStr, str, ok := utils.SplitString2(pattern, ":")
	if !ok {
		str = pattern
	}

	typ, ok := typMap[typStr]
	if !ok {
		return 0, "", fmt.Errorf("unexpected pattern type %s", typStr)
	}

	return typ, str, nil
}

func (m *MixMatcher) getSubMatcher(typ MixMatcherPatternType) Matcher {
	switch typ {
	case MixMatcherPatternTypeKeyword:
		return m.keyword
	case MixMatcherPatternTypeRegexp:
		return m.regex
	case MixMatcherPatternTypeDomain:
		return m.domain
	case MixMatcherPatternTypeFull:
		return m.full
	default:
		panic(fmt.Sprintf("MixMatcher: invalid type %d", typ))
	}
}
