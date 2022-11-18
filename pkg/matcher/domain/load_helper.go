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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"google.golang.org/protobuf/proto"
	"io"
	"strings"
	"unicode"
)

// ParseStringFunc parse data string to matcher pattern and additional attributions.
type ParseStringFunc[T any] func(s string) (pattern string, v T, err error)

// check if s only contains a domain pattern (no other section, no space).
func patternOnly[T any](s string) (pattern string, v T, err error) {
	if strings.IndexFunc(s, unicode.IsSpace) != -1 {
		return "", v, errors.New("rule string has more than one section")
	}
	return s, v, nil
}

// Load loads data from a string, parsing it with parseString function.
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

// NewDomainMixMatcher is a helper function for BatchLoadDomainProvider.
func NewDomainMixMatcher() *MixMatcher[struct{}] {
	mixMatcher := NewMixMatcher[struct{}]()
	mixMatcher.SetDefaultMatcher(MatcherDomain)
	return mixMatcher
}

type V2DomainPicker struct {
	tag   string
	attrs map[string]struct{}
}

// ParseV2Suffix parses s into a group of V2DomainPicker.
// The format of s is "tag[@attr@attr...],tag[@attr@attr...]..."
// Only domains that are matched by the tag AND has one of specified attrs will be picked up.
func ParseV2Suffix(s string) []*V2DomainPicker {
	vf := make([]*V2DomainPicker, 0)
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if len(t) == 0 {
			continue
		}
		s := strings.Split(t, "@")
		vf = append(vf, &V2DomainPicker{
			tag:   s[0],
			attrs: attrMap(s[1:]),
		})
	}
	return vf
}

// LoadFromGeoSite loads data from geosite package.
func LoadFromGeoSite(m *MixMatcher[struct{}], v *v2data.GeoSiteList, pickers ...*V2DomainPicker) error {
	dataTags := make(map[string][]*v2data.Domain)
	for _, gs := range v.GetEntry() {
		dataTags[strings.ToLower(gs.GetCountryCode())] = gs.Domain
	}

	for _, picker := range pickers {
		tag := picker.tag
		attrs := picker.attrs

		// Pick up tag.
		domains := dataTags[tag]
		if domains == nil {
			return fmt.Errorf("tag %s does not exist", tag)
		}
		_, err := pickUpAttrAndLoad(m, domains, attrs)
		if err != nil {
			return fmt.Errorf("failed to load tag %s, %w", tag, err)
		}
	}
	return nil
}

func pickUpAttrAndLoad(m *MixMatcher[struct{}], domains []*v2data.Domain, attrs map[string]struct{}) (*MixMatcher[struct{}], error) {
	for _, d := range domains {
		// check attrs if specified.
		if len(attrs) > 0 {
			hasAttr := false
			for _, attr := range d.Attribute {
				if _, ok := attrs[attr.Key]; ok {
					hasAttr = true
					break
				}
			}
			if !hasAttr {
				continue
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

func attrMap(attrs []string) map[string]struct{} {
	if len(attrs) == 0 {
		return nil
	}
	m := make(map[string]struct{})
	for _, attr := range attrs {
		m[attr] = struct{}{}
	}
	return m
}
