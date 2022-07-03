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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/data_provider"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"google.golang.org/protobuf/proto"
	"io"
	"strings"
	"sync"
)

// ParseStringFunc parse data string to matcher pattern and additional attributions.
type ParseStringFunc[T any] func(s string) (pattern string, v T, err error)

func patternOnly[T any](s string) (pattern string, v T, err error) {
	out := strings.Fields(s)
	if len(out) == 1 {
		return out[0], v, nil
	}
	return "", v, errors.New("string does not only contain pattern")
}

// Load loads data from a string. LoadFromText.
func Load[T any](m WriteableMatcher[T], s string, parseString ParseStringFunc[T]) error {
	if parseString == nil {
		parseString = patternOnly[T]
	}
	pattern, v, err := parseString(s)
	if err != nil {
		return err
	}
	return m.Add(pattern, v)
}

// BatchLoad loads multiple data strings using Load.
func BatchLoad[T any](m WriteableMatcher[T], b []string, parseString ParseStringFunc[T]) error {
	for _, s := range b {
		err := Load(m, s, parseString)
		if err != nil {
			return fmt.Errorf("failed to load data %s: %w", s, err)
		}
	}
	return nil
}

type MatcherGroup[T any] struct {
	g []Matcher[T]
}

func (m *MatcherGroup[T]) Match(s string) (v T, ok bool) {
	for _, sub := range m.g {
		v, ok = sub.Match(s)
		if ok {
			return v, true
		}
	}
	return
}

func (m *MatcherGroup[T]) Len() int {
	s := 0
	for _, sub := range m.g {
		s += sub.Len()
	}
	return s
}

func (m *MatcherGroup[T]) Append(nm Matcher[T]) {
	m.g = append(m.g, nm)
	return
}

// BatchLoadProvider loads multiple data entries.
func BatchLoadProvider[T any](
	e []string,
	staticMatcher WriteableMatcher[T],
	parseString ParseStringFunc[T],
	dm *data_provider.DataManager,
	parserFunc func(b []byte) (Matcher[T], error),
) (*MatcherGroup[T], error) {
	mg := new(MatcherGroup[T])
	mg.Append(staticMatcher)

	for _, s := range e {
		if strings.HasPrefix(s, "provider:") {
			providerTag := strings.TrimPrefix(s, "provider:")
			provider := dm.GetDataProvider(providerTag)
			if provider == nil {
				return nil, fmt.Errorf("cannot find provider %s", providerTag)
			}
			m := NewDynamicMatcher[T](parserFunc)
			if err := provider.LoadAndAddListener(m); err != nil {
				return nil, fmt.Errorf("failed to load data from provider %s, %w", providerTag, err)
			}
			mg.g = append(mg.g, m)
		} else {
			err := Load[T](staticMatcher, s, parseString)
			if err != nil {
				return nil, fmt.Errorf("failed to load data %s: %w", s, err)
			}
		}
	}
	return mg, nil
}

// BatchLoadDomainProvider loads multiple domain entries.
func BatchLoadDomainProvider(
	e []string,
	dm *data_provider.DataManager,
) (*MatcherGroup[struct{}], error) {
	mg := new(MatcherGroup[struct{}])
	staticMatcher := NewDomainMixMatcher()
	mg.Append(staticMatcher)
	for _, s := range e {
		if strings.HasPrefix(s, "provider:") {
			providerTag := strings.TrimPrefix(s, "provider:")
			providerTag, v2suffix, _ := strings.Cut(providerTag, ":")
			provider := dm.GetDataProvider(providerTag)
			if provider == nil {
				return nil, fmt.Errorf("cannot find provider %s", providerTag)
			}
			var parseFunc func(b []byte) (Matcher[struct{}], error)
			if len(v2suffix) > 0 {
				parseFunc = func(b []byte) (Matcher[struct{}], error) {
					return ParseV2rayDomainFile(b, ParseV2Suffix(v2suffix)...)
				}
			} else {
				parseFunc = func(b []byte) (Matcher[struct{}], error) {
					return ParseTextDomainFile(b)
				}
			}
			m := NewDynamicMatcher[struct{}](parseFunc)
			if err := provider.LoadAndAddListener(m); err != nil {
				return nil, fmt.Errorf("failed to load data from provider %s, %w", providerTag, err)
			}
			mg.g = append(mg.g, m)
		} else {
			err := Load[struct{}](staticMatcher, s, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to load data %s: %w", s, err)
			}
		}
	}
	return mg, nil
}

type DynamicMatcher[T any] struct {
	parserFunc func(b []byte) (Matcher[T], error)
	l          sync.RWMutex
	m          Matcher[T]
}

func NewDynamicMatcher[T any](parserFunc func(b []byte) (Matcher[T], error)) *DynamicMatcher[T] {
	return &DynamicMatcher[T]{parserFunc: parserFunc}
}

func (d *DynamicMatcher[T]) Match(s string) (v T, ok bool) {
	d.l.RLock()
	m := d.m
	d.l.RUnlock()
	return m.Match(s)
}

func (d *DynamicMatcher[T]) Len() int {
	d.l.RLock()
	m := d.m
	d.l.RUnlock()
	return m.Len()
}

func (d *DynamicMatcher[T]) Update(b []byte) error {
	m, err := d.parserFunc(b)
	if err != nil {
		return err
	}
	d.l.Lock()
	d.m = m
	d.l.Unlock()
	return nil
}

// LoadFromTextReader loads multiple lines from reader r. r
func LoadFromTextReader[T any](m WriteableMatcher[T], r io.Reader, parseString ParseStringFunc[T]) error {
	lineCounter := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineCounter++
		s := scanner.Text()
		s = utils.RemoveComment(s, "#")
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			continue
		}

		err := Load(m, s, parseString)
		if err != nil {
			return fmt.Errorf("line %d: %v", lineCounter, err)
		}
	}
	return scanner.Err()
}

// ParseV2rayDomainFile See NewV2rayDomainDat.
func ParseV2rayDomainFile(in []byte, filters ...*V2filter) (*MixMatcher[struct{}], error) {
	v, err := LoadGeoSiteList(in)
	if err != nil {
		return nil, err
	}
	return NewV2rayDomainDat(v, filters...)
}

type V2filter struct {
	Tag   string
	Attrs []string
}

// ParseV2Suffix parses s into V2filter.
// The format of s is "tag[@attr@attr...],tag[@attr@attr...]..."
func ParseV2Suffix(s string) []*V2filter {
	vf := make([]*V2filter, 0)
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if len(t) == 0 {
			continue
		}
		s := strings.Split(t, "@")
		tag := s[0]
		attr := s[1:]
		vf = append(vf, &V2filter{
			Tag:   tag,
			Attrs: attr,
		})
	}
	return vf
}

// NewV2rayDomainDat builds a V2rayDomainDat from given v and args.
// The format of args is "tag1@attr1@attr2,tag2@attr1...".
// Only domains that are matched by the args will be loaded to V2rayDomainDat.
func NewV2rayDomainDat(v *v2data.GeoSiteList, filters ...*V2filter) (*MixMatcher[struct{}], error) {
	dataTags := make(map[string][]*v2data.Domain)
	for _, gs := range v.GetEntry() {
		dataTags[strings.ToLower(gs.GetCountryCode())] = gs.Domain
	}

	m := NewMixMatcher[struct{}]()
	for _, f := range filters {
		tag := f.Tag
		attrs := f.Attrs
		domains := dataTags[tag]
		if domains == nil {
			return nil, fmt.Errorf("tag %s does not exist", tag)
		}
		_, err := BuildDomainMatcher(domains, attrs, m)
		if err != nil {
			return nil, fmt.Errorf("failed to load tag %s, %w", tag, err)
		}
	}

	return m, nil
}

func BuildDomainMatcher(domains []*v2data.Domain, attrs []string, m *MixMatcher[struct{}]) (*MixMatcher[struct{}], error) {
	am := make(map[string]struct{})
	if len(attrs) > 0 {
		for _, attr := range attrs {
			am[attr] = struct{}{}
		}
	}

	if m == nil {
		m = NewMixMatcher[struct{}]()
	}

getDomainLoop:
	for _, d := range domains {
		if len(am) > 0 {
			hasAttr := false
			for _, attr := range d.Attribute {
				if _, ok := am[attr.Key]; ok {
					hasAttr = true
					break
				}
			}
			if !hasAttr {
				continue getDomainLoop
			}
		}

		var subMatcherType string
		switch d.Type {
		case v2data.Domain_Plain:
			subMatcherType = MatcherKeyword
		case v2data.Domain_Regex:
			subMatcherType = MatcherRegexp
		case v2data.Domain_Domain:
			subMatcherType = MatcherDomain
		case v2data.Domain_Full:
			subMatcherType = MatcherFull
		default:
			return nil, fmt.Errorf("invalid v2ray Domain_Type %d", d.Type)
		}

		sm := m.GetSubMatcher(subMatcherType)
		if sm == nil {
			return nil, fmt.Errorf("invalid MixMatcher, missing submatcher %s", subMatcherType)
		}

		if err := sm.Add(d.Value, struct{}{}); err != nil {
			return nil, fmt.Errorf("failed to load value %s, %w", d.Value, err)
		}
	}
	return m, nil
}

func LoadGeoSiteList(b []byte) (*v2data.GeoSiteList, error) {
	geoSiteList := new(v2data.GeoSiteList)
	if err := proto.Unmarshal(b, geoSiteList); err != nil {
		return nil, err
	}
	return geoSiteList, nil
}

func ParseTextDomainFile(in []byte) (*MixMatcher[struct{}], error) {
	mixMatcher := NewDomainMixMatcher()
	if err := LoadFromTextReader[struct{}](mixMatcher, bytes.NewReader(in), nil); err != nil {
		return nil, err
	}
	return mixMatcher, nil
}

// NewDomainMixMatcher is a helper function for BatchLoadDomainProvider.
func NewDomainMixMatcher() *MixMatcher[struct{}] {
	mixMatcher := NewMixMatcher[struct{}]()
	mixMatcher.SetDefaultMatcher(MatcherDomain)
	return mixMatcher
}
