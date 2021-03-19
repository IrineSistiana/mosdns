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
	"strconv"
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

func Test_FullMatcher(t *testing.T) {
	m := NewFullMatcher()
	assert := assertFunc(t, m)
	add := func(fqdn string, v interface{}) {
		m.Add(fqdn, v)
	}

	add("cn.", nil)
	assert("cn.", true, nil)
	assert("a.cn.", false, nil)
	add("test.test.", nil)
	assert("test.test.", true, nil)
	assert("test.a.test.", false, nil)

	// test replace
	add("append.", 0)
	assert("append.", true, 0)
	add("append.", 1)
	assert("append.", true, 1)
	add("append.", nil)
	assert("append.", true, nil)

	// test appendable
	add("append.", nil)
	assert("append.", true, nil)
	add("append.", s("a"))
	assert("append.", true, s("a"))
	add("append.", s("b"))
	assert("append.", true, s("ab"))

	assertInt(t, m.Len(), 3)
}

func Test_KeywordMatcher(t *testing.T) {
	m := NewKeywordMatcher()
	add := func(fqdn string, v interface{}) {
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

	// test replace
	add("append.", 0)
	assert("append.", true, 0)
	add("append.", 1)
	assert("append.", true, 1)
	add("append.", nil)
	assert("append.", true, nil)

	// test appendable
	add("append.", nil)
	assert("a.append.", true, nil)
	add("append.", s("a"))
	assert("b.append.", true, s("a"))
	add("append.", s("b"))
	assert("c.append.", true, s("ab"))

	assertInt(t, m.Len(), 3)
}

func Test_RegexMatcher(t *testing.T) {
	m := NewRegexMatcher()
	add := func(expr string, v interface{}, wantErr bool) {
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

	// test replace
	add("append.", 0, false)
	assert("append.", true, 0)
	add("append.", 1, false)
	assert("append.", true, 1)
	add("append.", nil, false)
	assert("append.", true, nil)

	// test appendable
	expr = "append."
	add(expr, nil, false)
	assert("append.", true, nil)
	add(expr, s("a"), false)
	assert("a.append.", true, s("a"))
	add(expr, s("b"), false)
	assert("b.append.", true, s("ab"))

	expr = "*"
	add(expr, nil, true)
}

func Test_regCache(t *testing.T) {
	c := newRegCache(128)
	for i := 0; i < 1024; i++ {
		s := strconv.Itoa(i)
		res := new(regElem)
		c.cache(s, res)
		if len(c.m) > 128 {
			t.Fatal("cache overflowed")
		}
		got, ok := c.lookup(s)
		if !ok {
			t.Fatal("cache lookup failed")
		}
		if got != res {
			t.Fatal("cache item mismatched")
		}
	}
}
