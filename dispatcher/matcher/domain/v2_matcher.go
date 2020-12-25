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
	"strings"
	"v2ray.com/core/app/router"
)

type V2Matcher struct {
	dm *router.DomainMatcher
}

func (m *V2Matcher) Match(fqdn string) (v interface{}, ok bool) {
	domain := fqdn
	if strings.HasSuffix(domain, ".") {
		domain = domain[:len(domain)-1]
	}
	return nil, m.dm.ApplyDomain(domain)
}

func NewV2Matcher(domains []*router.Domain) (*V2Matcher, error) {
	dm, err := router.NewDomainMatcher(domains)
	if err != nil {
		return nil, err
	}
	return &V2Matcher{dm: dm}, nil
}

type MixMatcher struct {
	keyword *KeywordMatcher
	regex   *RegexMatcher
	domain  *DomainMatcher
	full    *DomainMatcher
}

func NewMixMatcher() *MixMatcher {
	return &MixMatcher{
		keyword: NewKeywordMatcher(),
		regex:   NewRegexMatcher(),
		domain:  NewDomainMatcher(MatchModeDomain),
		full:    NewDomainMatcher(MatchModeFull),
	}
}

func (m *MixMatcher) AddElem(typ router.Domain_Type, s string, v interface{}) error {
	switch typ {
	case router.Domain_Plain:
		m.keyword.Add(s, v)
	case router.Domain_Regex:
		err := m.regex.Add(s, v)
		if err != nil {
			return err
		}
	case router.Domain_Domain:
		m.domain.Add(s, v)
	case router.Domain_Full:
		m.full.Add(s, v)
	default:
		return fmt.Errorf("invalid type %d", typ)
	}
	return nil
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
