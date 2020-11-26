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
	l.Add("cn.")
	l.Add("a.com.")
	l.Add("b.com.")
	l.Add("abc.com.")
	l.Add("123456789012345678901234567890.com.")

	assertTrue(l.Match("a.cn."))
	assertTrue(l.Match("a.b.cn."))

	assertTrue(l.Match("a.com."))
	assertTrue(l.Match("b.com."))
	assertTrue(!l.Match("c.com."))
	assertTrue(!l.Match("a.c.com."))
	assertTrue(l.Match("123456789012345678901234567890.com."))

	assertTrue(l.Match("abc.abc.com."))
}

func assertTrue(b bool) {
	if !b {
		panic("assert failed")
	}
}
