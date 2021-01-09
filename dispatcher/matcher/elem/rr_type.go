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

package elem

type IntMatcher struct {
	m map[int]struct{}
}

// NewIntMatcher inits a new IntMatcher.
func NewIntMatcher(elem []int) *IntMatcher {
	matcher := &IntMatcher{m: make(map[int]struct{})}

	for _, v := range elem {
		matcher.m[v] = struct{}{}
	}
	return matcher
}

func (m *IntMatcher) Match(v int) bool {
	_, ok := m.m[v]
	return ok
}
