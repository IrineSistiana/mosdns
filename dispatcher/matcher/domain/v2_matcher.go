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
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/v2data"
)

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

func (m *MixMatcher) AddElem(typ v2data.Domain_Type, s string, v Appendable) error {
	switch typ {
	case v2data.Domain_Plain:
		m.keyword.Add(s, v)
	case v2data.Domain_Regex:
		err := m.regex.Add(s, v)
		if err != nil {
			return err
		}
	case v2data.Domain_Domain:
		m.domain.Add(s, v)
	case v2data.Domain_Full:
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

func (m *MixMatcher) Len() int {
	sum := 0
	for _, m := range [...]Matcher{m.domain, m.keyword, m.regex, m.full} {
		sum += m.Len()
	}
	return sum
}
