/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package domain

import (
	"reflect"
	"testing"
)

func assertFunc[T any](t *testing.T, m Matcher[T]) func(domain string, wantBool bool, wantV any) {
	return func(domain string, wantBool bool, wantV any) {
		t.Helper()
		v, ok := m.Match(domain)
		if ok != wantBool {
			t.Fatalf("%s, wantBool = %v, got = %v", domain, wantBool, ok)
		}

		if !reflect.DeepEqual(v, wantV) {
			t.Fatalf("%s, wantV = %v, got = %v", domain, wantV, v)
		}
	}
}

type aStr struct {
	s string
}

func s(str string) *aStr {
	return &aStr{s: str}
}

func (a *aStr) Append(v any) {
	a.s = a.s + v.(*aStr).s
}

func TestDomainMatcher(t *testing.T) {
	m := NewSubDomainMatcher[any]()
	add := func(domain string, v any) {
		m.Add(domain, v)
	}
	assert := assertFunc[any](t, m)

	add("cn", nil)
	assertInt(t, 1, m.Len())
	assert("cn", true, nil)
	assert("a.cn.", true, nil)
	assert("a.com", false, nil)
	add("a.b.com", nil)
	assertInt(t, 2, m.Len())
	assert("a.b.com.", true, nil)
	assert("q.w.e.r.a.b.com.", true, nil)
	assert("b.com.", false, nil)

	// test replace
	add("append", 0)
	assertInt(t, 3, m.Len())
	assert("append.", true, 0)
	add("append.", 1)
	assert("append.", true, 1)
	add("append", nil)
	assert("append.", true, nil)

	// test sub domain
	add("sub", 1)
	assertInt(t, 4, m.Len())
	add("a.sub", 2)
	assertInt(t, 5, m.Len())
	assert("sub", true, 1)
	assert("b.sub", true, 1)
	assert("a.sub", true, 2)
	assert("a.a.sub", true, 2)

	// test case-insensitive
	add("UPpER", 1)
	assert("LowER.Upper", true, 1)

	// root match
	add(".", 9)
	assert("any.domain", true, 9)
}

func assertInt(t testing.TB, want, got int) {
	t.Helper()
	if want != got {
		t.Errorf("assertion failed: want %d, got %d", want, got)
	}
}

func Test_FullMatcher(t *testing.T) {
	m := NewFullMatcher[any]()
	assert := assertFunc[any](t, m)
	add := func(domain string, v any) {
		m.Add(domain, v)
	}

	add("cn", nil)
	assert("cn", true, nil)
	assert("a.cn", false, nil)
	add("test.test", nil)
	assert("test.test", true, nil)
	assert("test.a.test", false, nil)

	// test replace
	add("append", 0)
	assert("append", true, 0)
	add("append", 1)
	assert("append", true, 1)
	add("append", nil)
	assert("append", true, nil)

	assertInt(t, m.Len(), 3)

	// test case-insensitive
	add("UPpER", 1)
	assert("Upper", true, 1)
}

func Test_KeywordMatcher(t *testing.T) {
	m := NewKeywordMatcher[any]()
	add := func(domain string, v any) {
		m.Add(domain, v)
	}

	assert := assertFunc[any](t, m)

	add("123", s("a"))
	assert("123456.cn", true, s("a"))
	assert("111123.com", true, s("a"))
	assert("111111.cn", false, nil)
	add("example.com", nil)
	assert("sub.example.com", true, nil)
	assert("example_sub.com", false, nil)

	// test replace
	add("append", 0)
	assert("append", true, 0)
	add("append", 1)
	assert("append", true, 1)
	add("append", nil)
	assert("append", true, nil)

	assertInt(t, m.Len(), 3)

	// test case-insensitive
	add("UPpER", 1)
	assert("L.Upper.U", true, 1)
}

func Test_RegexMatcher(t *testing.T) {
	m := NewRegexMatcher[any]()
	add := func(expr string, v any, wantErr bool) {
		err := m.Add(expr, v)
		if (err != nil) != wantErr {
			t.Fatalf("%s: want err %v, got %v", expr, wantErr, err != nil)
		}
	}

	assert := assertFunc[any](t, m)

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

	// test replace
	add("append", 0, false)
	assert("append", true, 0)
	add("append", 1, false)
	assert("append", true, 1)
	add("append", nil, false)
	assert("append", true, nil)

	expr = "*"
	add(expr, nil, true)
}
