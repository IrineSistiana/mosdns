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
	"testing"
)

func Test_DomainList(t *testing.T) {
	l := NewListMatcher()

	add := func(fqdn string) {
		l.Add(fqdn, fqdn)
	}
	assertMatched := func(fqdn, base string) {
		v, ok := l.Match(fqdn)
		if !ok || base != v.(string) {
			t.Fatal()
		}
	}
	assertNotMatched := func(fqdn string) {
		v, ok := l.Match(fqdn)
		if ok || v != nil {
			t.Fatal()
		}
	}
	add("cn.")
	add("a.com.")
	add("b.com.")
	add("abc.com.")
	add("123456789012345678901234567890.com.")

	assertMatched("a.cn.", "cn.")
	assertMatched("a.b.cn.", "cn.")
	assertMatched("a.com.", "a.com.")
	assertMatched("b.com.", "b.com.")
	assertMatched("abc.abc.com.", "abc.com.")
	assertMatched("123456.123456789012345678901234567890.com.", "123456789012345678901234567890.com.")

	assertNotMatched("us.")
	assertNotMatched("c.com.")
	assertNotMatched("a.c.com.")
}
