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
	"context"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/dnsutils"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/matcher/domain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/matcher/elem"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/matcher/netlist"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/miekg/dns"
	"net"
	"net/netip"
	"testing"
)

func TestClientIPMatcher_Match(t *testing.T) {
	type fields struct {
		ipMatcher netlist.Matcher
	}
	type args struct {
		qCtx *query_context.Context
	}

	nl := netlist.NewList()
	if err := netlist.LoadFromText(nl, "127.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	nl.Sort()

	msg := new(dns.Msg)
	meta1271 := &query_context.RequestMeta{ClientAddr: netip.MustParseAddr("127.0.0.1")}
	meta1281 := &query_context.RequestMeta{ClientAddr: netip.MustParseAddr("128.0.0.1")}
	metaNilAddr := &query_context.RequestMeta{}

	tests := []struct {
		name        string
		fields      fields
		args        args
		wantMatched bool
		wantErr     bool
	}{
		{"matched", fields{ipMatcher: nl}, args{query_context.NewContext(msg, meta1271)}, true, false},
		{"not matched", fields{ipMatcher: nl}, args{query_context.NewContext(msg, meta1281)}, false, false},
		{"no meta", fields{ipMatcher: nl}, args{query_context.NewContext(msg, nil)}, false, false},
		{"no addr", fields{ipMatcher: nl}, args{query_context.NewContext(msg, metaNilAddr)}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewClientIPMatcher(tt.fields.ipMatcher)
			gotMatched, err := m.Match(context.Background(), tt.args.qCtx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Match() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotMatched != tt.wantMatched {
				t.Errorf("Match() gotMatched = %v, want %v", gotMatched, tt.wantMatched)
			}
		})
	}
}

func TestClientECSMatcher_Match(t *testing.T) {
	nl := netlist.NewList()
	if err := netlist.LoadFromText(nl, "127.0.0.0/24"); err != nil {
		t.Fatal(err)
	}
	nl.Sort()

	msg := new(dns.Msg)
	msgWithoutOPT := msg
	msg = new(dns.Msg)
	msg.SetEdns0(512, false)
	msgWithOPT := msg
	msg = new(dns.Msg)
	msg.SetEdns0(512, false)
	opt := msg.IsEdns0()
	dnsutils.AddECS(opt, &dns.EDNS0_SUBNET{Address: net.ParseIP("127.0.0.1")}, false)
	msg1271 := msg
	msg1281 := msg.Copy()
	opt = msg1281.IsEdns0()
	dnsutils.AddECS(opt, &dns.EDNS0_SUBNET{Address: net.ParseIP("128.0.0.1")}, true)

	tests := []struct {
		name        string
		matcher     netlist.Matcher
		qCtx        *query_context.Context
		wantMatched bool
		wantErr     bool
	}{
		{"matched", nl, query_context.NewContext(msg1271, nil), true, false},
		{"not matched", nl, query_context.NewContext(msg1281, nil), false, false},
		{"no ecs", nl, query_context.NewContext(msgWithOPT, nil), false, false},
		{"no opt", nl, query_context.NewContext(msgWithoutOPT, nil), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewClientECSMatcher(tt.matcher)
			gotMatched, err := m.Match(context.Background(), tt.qCtx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Match() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotMatched != tt.wantMatched {
				t.Errorf("Match() gotMatched = %v, want %v", gotMatched, tt.wantMatched)
			}
		})
	}
}
func TestQNameMatcher_Match(t *testing.T) {
	dm := domain.NewSubDomainMatcher[struct{}]()
	dm.Add("com.", struct{}{})

	qm := NewQNameMatcher(dm)
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	if !qm.MatchMsg(m) {
		t.Fatal()
	}

	m.SetQuestion("example.xxx.", dns.TypeA)
	if qm.MatchMsg(m) {
		t.Fatal()
	}
}

func TestQTypeMatcher_Match(t *testing.T) {
	em := elem.NewIntMatcher([]int{int(dns.TypeA)})
	qm := NewQTypeMatcher(em)
	m := new(dns.Msg)
	m.SetQuestion(".", dns.TypeA)
	if !qm.MatchMsg(m) {
		t.Fatal()
	}

	m.SetQuestion(".", dns.TypeAAAA)
	if qm.MatchMsg(m) {
		t.Fatal()
	}
}

func TestQClassMatcher_Match(t *testing.T) {
	em := elem.NewIntMatcher([]int{dns.ClassINET})
	qm := NewQClassMatcher(em)
	m := new(dns.Msg)
	m.Question = []dns.Question{{Name: ".", Qtype: dns.TypeA, Qclass: dns.ClassINET}}
	if !qm.MatchMsg(m) {
		t.Fatal()
	}

	m.Question = []dns.Question{{Name: ".", Qtype: dns.TypeA, Qclass: dns.ClassANY}}
	if qm.MatchMsg(m) {
		t.Fatal()
	}
}
