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
	"strings"
	"v2ray.com/core/app/router"
)

type V2Matcher struct {
	dm *router.DomainMatcher
}

func (m *V2Matcher) Match(fqdn string) (v interface{}, ok bool) {
	if strings.HasSuffix(fqdn, ".") {
		fqdn = fqdn[:len(fqdn)-1]
	}
	return nil, m.dm.ApplyDomain(fqdn)
}

func NewV2Matcher(domains []*router.Domain) (*V2Matcher, error) {
	dm, err := router.NewDomainMatcher(domains)
	if err != nil {
		return nil, err
	}
	return &V2Matcher{dm: dm}, nil
}
