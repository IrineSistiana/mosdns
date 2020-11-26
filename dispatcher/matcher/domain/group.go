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

type MatcherGroup struct {
	m []Matcher
}

func (mg *MatcherGroup) Match(fqdn string) bool {
	for _, m := range mg.m {
		if m.Match(fqdn) {
			return true
		}
	}
	return false
}

func NewMatcherGroup(m []Matcher) *MatcherGroup {
	return &MatcherGroup{m: m}
}
