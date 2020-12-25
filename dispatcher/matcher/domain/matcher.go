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
	"fmt"
	"github.com/miekg/dns"
	"regexp"
	"strings"
)

type DomainMatcher struct {
	mode MatchMode

	s map[[16]byte]interface{}
	m map[[32]byte]interface{}
	l map[[256]byte]interface{}
}

type MatchMode uint8

const (
	MatchModeDomain MatchMode = iota
	MatchModeFull
)

func NewDomainMatcher(mode MatchMode) *DomainMatcher {
	return &DomainMatcher{
		mode: mode,
		s:    make(map[[16]byte]interface{}),
		m:    make(map[[32]byte]interface{}),
		l:    make(map[[256]byte]interface{}),
	}
}

func (m *DomainMatcher) Add(domain string, v interface{}) {
	fqdn := dns.Fqdn(domain)
	n := len(fqdn)

	switch {
	case n <= 16:
		var b [16]byte
		copy(b[:], fqdn)
		m.s[b] = v
	case n <= 32:
		var b [32]byte
		copy(b[:], fqdn)
		m.m[b] = v
	default:
		var b [256]byte
		copy(b[:], fqdn)
		m.l[b] = v
	}
}

func (m *DomainMatcher) Match(fqdn string) (v interface{}, ok bool) {
	switch m.mode {
	case MatchModeFull:
		return m.fullMatch(fqdn)
	case MatchModeDomain:
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
	kws []*kwElem
}

type kwElem struct {
	s string
	v interface{}
}

func NewKeywordMatcher() *KeywordMatcher {
	return &KeywordMatcher{
		kws: make([]*kwElem, 0),
	}
}

func (m *KeywordMatcher) Add(keyword string, v interface{}) {
	m.kws = append(m.kws, &kwElem{s: keyword, v: v})
}

func (m *KeywordMatcher) Match(fqdn string) (v interface{}, ok bool) {
	for _, e := range m.kws {
		if strings.Contains(fqdn, e.s) {
			return e.v, true
		}
	}
	return nil, false
}

type RegexMatcher struct {
	regs []*regElem
}

type regElem struct {
	reg *regexp.Regexp
	v   interface{}
}

func NewRegexMatcher() *RegexMatcher {
	return &RegexMatcher{regs: make([]*regElem, 0)}
}

func (m *RegexMatcher) Add(expr string, v interface{}) error {
	reg, err := regexp.Compile(expr)
	if err != nil {
		return err
	}
	m.regs = append(m.regs, &regElem{reg: reg, v: v})
	return nil
}

func (m *RegexMatcher) Match(fqdn string) (v interface{}, ok bool) {
	for _, e := range m.regs {
		if e.reg.MatchString(fqdn) {
			return e.v, true
		}
	}
	return nil, false
}

type MatcherGroup struct {
	m []Matcher
}

func (mg *MatcherGroup) Match(fqdn string) (v interface{}, ok bool) {
	for _, m := range mg.m {
		if v, ok = m.Match(fqdn); ok {
			return
		}
	}
	return nil, false
}

func NewMatcherGroup(m []Matcher) *MatcherGroup {
	return &MatcherGroup{m: m}
}
