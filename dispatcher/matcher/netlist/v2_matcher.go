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

package netlist

import (
	"net"
	"v2ray.com/core/app/router"
)

type V2Matcher struct {
	m *router.GeoIPMatcher
}

func (m *V2Matcher) Match(ip net.IP) bool {
	return m.m.Match(ip)
}

func NewV2Matcher(cidrs []*router.CIDR) (*V2Matcher, error) {
	m := new(router.GeoIPMatcher)
	err := m.Init(cidrs)
	if err != nil {
		return nil, err
	}
	return &V2Matcher{m: m}, nil
}
