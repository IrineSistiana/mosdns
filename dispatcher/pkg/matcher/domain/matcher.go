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
	"sync"
)

type FullMatcher struct {
	m map[string]interface{}
}

func NewFullMatcher() *FullMatcher {
	return &FullMatcher{
		m: make(map[string]interface{}),
	}
}

func (m *FullMatcher) Add(domain string, v interface{}) error {
	m.add(domain, v)
	return nil
}

func (m *FullMatcher) add(domain string, v interface{}) {
	fqdn := dns.Fqdn(domain)
	oldV := m.m[fqdn]
	if appendable, ok := oldV.(Appendable); ok {
		appendable.Append(v)
	} else {
		m.m[fqdn] = v
	}
}

func (m *FullMatcher) Match(fqdn string) (v interface{}, ok bool) {
	v, ok = m.m[fqdn]
	return
}

func (m *FullMatcher) Len() int {
	return len(m.m)
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
	if appendable, ok := o.(Appendable); ok {
		appendable.Append(v)
	} else {
		m.kws[keyword] = v
	}
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
	regs  map[string]*regElem
	cache *regCache
}

type regElem struct {
	reg *regexp.Regexp
	v   interface{}
}

func NewRegexMatcher() *RegexMatcher {
	return &RegexMatcher{regs: make(map[string]*regElem)}
}

func NewRegexMatcherWithCache(cap int) *RegexMatcher {
	return &RegexMatcher{regs: make(map[string]*regElem), cache: newRegCache(cap)}
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
	} else {
		if appendable, ok := e.v.(Appendable); ok {
			appendable.Append(v)
		} else {
			e.v = v
		}
	}

	return nil
}

func (m *RegexMatcher) Match(fqdn string) (v interface{}, ok bool) {
	if m.cache != nil {
		if e, ok := m.cache.lookup(fqdn); ok { // cache hit
			if e != nil {
				return e.v, true // matched
			}
			return nil, false // not matched
		}
	}

	for _, e := range m.regs {
		if e.reg.MatchString(fqdn) {
			if m.cache != nil {
				m.cache.cache(fqdn, e)
			}
			return e.v, true
		}
	}

	if m.cache != nil { // cache the string
		m.cache.cache(fqdn, nil)
	}
	return nil, false
}

func (m *RegexMatcher) Len() int {
	return len(m.regs)
}

func (m *RegexMatcher) ResetCache() {
	if m.cache != nil {
		m.cache.reset()
	}
}

type regCache struct {
	cap int
	sync.RWMutex
	m map[string]*regElem
}

func newRegCache(cap int) *regCache {
	return &regCache{
		cap: cap,
		m:   make(map[string]*regElem, cap),
	}
}

func (c *regCache) cache(s string, res *regElem) {
	c.Lock()
	defer c.Unlock()

	c.tryEvictUnderLock()
	c.m[s] = res
}

func (c *regCache) lookup(s string) (res *regElem, ok bool) {
	c.RLock()
	defer c.RUnlock()
	res, ok = c.m[s]
	return
}

func (c *regCache) reset() {
	c.Lock()
	defer c.Unlock()
	c.m = make(map[string]*regElem, c.cap)
}

func (c *regCache) tryEvictUnderLock() {
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
	full    *FullMatcher
}

func NewMixMatcher() *MixMatcher {
	return &MixMatcher{
		keyword: NewKeywordMatcher(),
		regex:   NewRegexMatcherWithCache(4096),
		domain:  NewDomainMatcher(),
		full:    NewFullMatcher(),
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

func (m *MixMatcher) Match(fqdn string) (v interface{}, ok bool) {
	// it seems v2ray match full matcher first, then domain, reg and keyword matcher.
	for _, matcher := range [...]Matcher{m.full, m.domain, m.regex, m.keyword} {
		if v, ok = matcher.Match(fqdn); ok {
			return
		}
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
