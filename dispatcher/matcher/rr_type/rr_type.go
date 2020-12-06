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

package rr_type

type Matcher struct {
	e map[uint16]struct{}
}

// NewMatcher inits a new RR type matcher.
func NewMatcher(types []uint16) *Matcher {
	matcher := &Matcher{e: make(map[uint16]struct{})}

	for _, typ := range types {
		matcher.e[typ] = struct{}{}
	}
	return matcher
}

func (m *Matcher) Match(t uint16) bool {
	_, ok := m.e[t]
	return ok
}
