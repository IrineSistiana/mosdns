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
	"reflect"
	"testing"
)

func assertFunc(t *testing.T, m Matcher) func(fqdn string, wantBool bool, wantV interface{}) {
	return func(fqdn string, wantBool bool, wantV interface{}) {
		v, ok := m.Match(fqdn)
		if ok != wantBool {
			t.Fatalf("%s, wantBool = %v, got = %v", fqdn, wantBool, ok)
		}

		if !reflect.DeepEqual(v, wantV) {
			t.Fatalf("%s, wantV = %v, got = %v", fqdn, wantV, v)
		}
	}
}

type aStr struct {
	s string
}

func s(str string) *aStr {
	return &aStr{s: str}
}

func (a *aStr) Append(v interface{}) {
	a.s = a.s + v.(*aStr).s
}

func Test_DomainMatcher(t *testing.T) {
	m := NewDomainMatcher(MatchModeDomain)
	add := func(fqdn string, v Appendable) {
		m.Add(fqdn, v)
	}
	assert := assertFunc(t, m)

	add("cn.", nil)
	assert("cn.", true, nil)
	assert("a.cn.", true, nil)
	assert("a.com.", false, nil)
	add("a.b.com.", nil)
	assert("a.b.com.", true, nil)
	assert("q.w.e.r.a.b.com.", true, nil)
	assert("c.a.com.", false, nil)

	// test appendable
	add("append.", nil)
	assert("a.append.", true, nil)
	add("append.", s("a"))
	assert("b.append.", true, s("a"))
	add("append.", s("b"))
	assert("c.append.", true, s("ab"))

	m = NewDomainMatcher(MatchModeFull)
	assert = assertFunc(t, m)

	add("cn.", nil)
	assert("cn.", true, nil)
	assert("a.cn.", false, nil)
	add("test.test.", nil)
	assert("test.test.", true, nil)
	assert("test.a.test.", false, nil)
}

func Test_KeywordMatcher(t *testing.T) {
	m := NewKeywordMatcher()
	add := func(fqdn string, v Appendable) {
		m.Add(fqdn, v)
	}

	assert := assertFunc(t, m)

	add("123", s("a"))
	assert("123456.cn.", true, s("a"))
	assert("111123.com.", true, s("a"))
	assert("111111.cn.", false, nil)
	add("example.com", nil)
	assert("sub.example.com.", true, nil)
	assert("example_sub.com.", false, nil)

	// test appendable
	add("append.", nil)
	assert("a.append.", true, nil)
	add("append.", s("a"))
	assert("b.append.", true, s("a"))
	add("append.", s("b"))
	assert("c.append.", true, s("ab"))
}

func Test_RegexMatcher(t *testing.T) {
	m := NewRegexMatcher()
	add := func(expr string, v Appendable, wantErr bool) {
		err := m.Add(expr, v)
		if (err != nil) != wantErr {
			t.Fatalf("%s: want err %v, got %v", expr, wantErr, err != nil)
		}
	}

	assert := assertFunc(t, m)

	expr := "^github-production-release-asset-[0-9a-za-z]{6}\\.s3\\.amazonaws\\.com$"
	add(expr, nil, false)
	assert("github-production-release-asset-000000.s3.amazonaws.com", true, nil)
	assert("github-production-release-asset-aaaaaa.s3.amazonaws.com", true, nil)
	assert("github-production-release-asset-aa.s3.amazonaws.com", false, nil)
	assert("prefix_github-production-release-asset-000000.s3.amazonaws.com", false, nil)
	assert("github-production-release-asset-000000.s3.amazonaws.com.suffix", false, nil)

	expr = "^example"
	add(expr, nil, false)
	assert("example.com", true, nil)
	assert("sub.example.com", false, nil)

	// test appendable
	expr = "append.$"
	add(expr, nil, false)
	assert("append.", true, nil)
	add(expr, s("a"), false)
	assert("a.append.", true, s("a"))
	add(expr, s("b"), false)
	assert("b.append.", true, s("ab"))

	expr = "*"
	add(expr, nil, true)
}
