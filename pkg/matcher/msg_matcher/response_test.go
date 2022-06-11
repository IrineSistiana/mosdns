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

package msg_matcher

import (
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/netlist"
	"github.com/miekg/dns"
	"net"
	"testing"
)

func TestAAAAAIPMatcher_MatchMsg(t *testing.T) {
	nl := netlist.NewList()
	if err := netlist.LoadFromText(nl, "127.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	nl.Sort()
	m := NewAAAAAIPMatcher(nl)

	ip1271 := net.ParseIP("127.0.0.1")
	ip1281 := net.ParseIP("128.0.0.1")

	msg := new(dns.Msg)
	msg.Answer = []dns.RR{&dns.A{A: ip1281}, &dns.A{A: ip1271}}
	if matched, err := m.MatchMsg(msg); !matched || err != nil {
		t.Fatal()
	}

	msg.Answer = []dns.RR{&dns.A{A: ip1281}}
	if matched, err := m.MatchMsg(msg); matched || err != nil {
		t.Fatal()
	}

	msg.Answer = []dns.RR{}
	if matched, err := m.MatchMsg(msg); matched || err != nil {
		t.Fatal()
	}
}
