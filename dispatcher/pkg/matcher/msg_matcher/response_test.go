package msg_matcher

import (
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/netlist"
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
