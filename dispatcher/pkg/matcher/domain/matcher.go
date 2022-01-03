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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"regexp"
	"strings"
	"sync"
)

type FullMatcher struct {
	m map[string]interface{} // string must be a fqdn.
}

func NewFullMatcher() *FullMatcher {
	return &FullMatcher{
		m: make(map[string]interface{}),
	}
}

func (m *FullMatcher) Add(s string, v interface{}) error {
	domain := UnifyDomain(s)
	m.add(domain, v)
	return nil
}

func (m *FullMatcher) add(domain string, v interface{}) {
	oldV := m.m[domain]
	if appendable, ok := oldV.(Appendable); ok {
		appendable.Append(v)
	} else {
		m.m[domain] = v
	}
}

func (m *FullMatcher) Match(s string) (v interface{}, ok bool) {
	v, ok = m.m[UnifyDomain(s)]
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

func (m *KeywordMatcher) Match(s string) (v interface{}, ok bool) {
	domain := UnifyDomain(s)
	for k, v := range m.kws {
		if strings.Contains(domain, k) {
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

func (m *RegexMatcher) Match(s string) (v interface{}, ok bool) {
	return m.match(TrimDot(s))
}

func (m *RegexMatcher) match(domain string) (v interface{}, ok bool) {
	if m.cache != nil {
		if e, ok := m.cache.lookup(domain); ok { // cache hit
			if e != nil {
				return e.v, true // matched
			}
			return nil, false // not matched
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
	typMap map[string]MixMatcherPatternType // Default is MixMatcherStrToPatternTypeDefaultDomain

	// lazy init by getSubMatcher
	full    Matcher
	domain  Matcher
	keyword Matcher
	regex   Matcher
}

// NewMixMatcher creates a MixMatcher.
func NewMixMatcher(options ...MixMatcherOption) *MixMatcher {
	m := &MixMatcher{} // lazy init
	for _, f := range options {
		f(m)
	}
	return m
}

type MixMatcherOption func(mm *MixMatcher)

func WithFullMatcher(m Matcher) MixMatcherOption {
	return func(mm *MixMatcher) {
		mm.full = m
	}
}

func WithKeywordMatcher(m Matcher) MixMatcherOption {
	return func(mm *MixMatcher) {
		mm.keyword = m
	}
}

func WithDomainMatcher(m Matcher) MixMatcherOption {
	return func(mm *MixMatcher) {
		mm.domain = m
	}
}

func WithRegexpMatcher(m Matcher) MixMatcherOption {
	return func(mm *MixMatcher) {
		mm.regex = m
	}
}

// NewMixMatcherFrom creates a MixMatcher from those sub matcher.
// It ok to pass nil Matcher here. MixMatcher can lazy init nil sub matcher.
func NewMixMatcherFrom(full, domain, keyword, regex Matcher) *MixMatcher {
	return &MixMatcher{
		full:    full,
		domain:  domain,
		keyword: keyword,
		regex:   regex,
	}
}

var MixMatcherStrToPatternTypeDefaultDomain = map[string]MixMatcherPatternType{
	"":        MixMatcherPatternTypeDomain,
	"domain":  MixMatcherPatternTypeDomain,
	"keyword": MixMatcherPatternTypeKeyword,
	"regexp":  MixMatcherPatternTypeRegexp,
	"full":    MixMatcherPatternTypeFull,
}

var MixMatcherStrToPatternTypeDefaultFull = map[string]MixMatcherPatternType{
	"domain":  MixMatcherPatternTypeDomain,
	"keyword": MixMatcherPatternTypeKeyword,
	"regexp":  MixMatcherPatternTypeRegexp,
	"full":    MixMatcherPatternTypeFull,
	"":        MixMatcherPatternTypeFull,
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

func (m *MixMatcher) Match(s string) (v interface{}, ok bool) {
	for _, matcher := range [...]Matcher{m.full, m.domain, m.regex, m.keyword} {
		if matcher == nil {
			continue
		}
		if v, ok = matcher.Match(s); ok {
			return
		}
	}
	return
}

func (m *MixMatcher) Len() int {
	sum := 0
	for _, matcher := range [...]Matcher{m.domain, m.keyword, m.regex, m.full} {
		if matcher == nil {
			continue
		}
		sum += matcher.Len()
	}
	return sum
}

func (m *MixMatcher) splitTypeAndPattern(pattern string) (MixMatcherPatternType, string, error) {
	typMap := m.typMap
	if typMap == nil {
		typMap = MixMatcherStrToPatternTypeDefaultDomain
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
		if m.keyword == nil {
			m.keyword = NewKeywordMatcher()
		}
		return m.keyword
	case MixMatcherPatternTypeRegexp:
		if m.regex == nil {
			m.regex = NewRegexMatcher()
		}
		return m.regex
	case MixMatcherPatternTypeDomain:
		if m.domain == nil {
			m.domain = NewDomainMatcher()
		}
		return m.domain
	case MixMatcherPatternTypeFull:
		if m.full == nil {
			m.full = NewFullMatcher()
		}
		return m.full
	default:
		panic(fmt.Sprintf("MixMatcher: invalid type %d", typ))
	}
}

// TrimDot trims the suffix '.'.
func TrimDot(s string) string {
	return strings.TrimSuffix(s, ".")
}

// UnifyDomain unifies domain strings.
// It remove the suffix "." and make sure the domain is in lower case.
func UnifyDomain(s string) string {
	return strings.ToLower(TrimDot(s))
}
