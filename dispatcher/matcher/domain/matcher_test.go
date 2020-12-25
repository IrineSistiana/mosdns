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
	"reflect"
	"testing"
)

func assertFunc(t *testing.T, m Matcher) func(fqdn string, wantBool bool, wantV interface{}) {
	return func(fqdn string, wantBool bool, wantV interface{}) {
		v, ok := m.Match(fqdn)
		if ok != wantBool || !reflect.DeepEqual(v, wantV) {
			t.Fatal()
		}
	}
}

func Test_DomainMatcher(t *testing.T) {
	m := NewDomainMatcher(MatchModeDomain)
	add := func(fqdn string) {
		m.Add(fqdn, fqdn)
	}
	assertMatched := func(fqdn, base string) {
		v, ok := m.Match(fqdn)
		if !ok || base != v.(string) {
			t.Fatal()
		}
	}
	assertNotMatched := func(fqdn string) {
		v, ok := m.Match(fqdn)
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

	m.mode = MatchModeFull
	assertNotMatched("a.cn.")
	assertMatched("a.com.", "a.com.")
	assertNotMatched("b.a.com.")
}

func Test_KeywordMatcher(t *testing.T) {
	m := NewKeywordMatcher()
	add := func(keyword string) {
		m.Add(keyword, keyword)
	}

	assert := assertFunc(t, m)

	add("123")
	assert("123456.cn.", true, "123")
	assert("456.cn.", false, nil)
	add("example.com")
	assert("sub.example.com.", true, "example.com")
	assert("example_sub.com.", false, nil)
}

func Test_RegexMatcher(t *testing.T) {
	m := NewRegexMatcher()
	add := func(expr string, wantErr bool) {
		err := m.Add(expr, expr)
		if err != nil && !wantErr {
			t.Fatal(err)
		}
	}

	assert := assertFunc(t, m)

	s := "^github-production-release-asset-[0-9a-za-z]{6}\\.s3\\.amazonaws\\.com$"
	add(s, false)
	assert("github-production-release-asset-000000.s3.amazonaws.com", true, s)
	assert("github-production-release-asset-aaaaaa.s3.amazonaws.com", true, s)
	assert("github-production-release-asset-aa.s3.amazonaws.com", false, nil)
	assert("prefix_github-production-release-asset-000000.s3.amazonaws.com", false, nil)
	assert("github-production-release-asset-000000.s3.amazonaws.com.suffix", false, nil)

	s = "^example"
	add(s, false)
	assert("example.com", true, s)
	assert("sub.example.com", false, nil)

	s = "*"
	add(s, true)
}
