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

package zone_file

import (
	"github.com/miekg/dns"
	"strings"
	"testing"
)

const data = `
$TTL 3600
example.com.  IN  A     192.0.2.1
1.example.com.  IN  AAAA     2001:db8:10::1
`

func TestMatcher(t *testing.T) {
	m := new(Matcher)
	err := m.Load(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	r := m.Reply(q)
	if r == nil {
		t.Fatal("search failed")
	}
	if got := r.Answer[0].(*dns.A).A.String(); got != "192.0.2.1" {
		t.Fatalf("want ip 192.0.2.1, got %s", got)
	}

	q = new(dns.Msg)
	q.SetQuestion("1.example.com.", dns.TypeAAAA)
	r = m.Reply(q)
	if r == nil {
		t.Fatal("search failed")
	}
	if got := r.Answer[0].(*dns.AAAA).AAAA.String(); got != "2001:db8:10::1" {
		t.Fatalf("want ip 2001:db8:10::1, got %s", got)
	}
}
