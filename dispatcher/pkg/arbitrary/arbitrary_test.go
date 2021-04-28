package arbitrary

import (
	"github.com/miekg/dns"
	"net"
	"strconv"
	"testing"
)

func Test_strToUint16(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want uint16
		ok   bool
	}{
		{"-1", "-1", 0, false},
		{"0", "0", 0, true},
		{"10000", "10000", 10000, true},
		{"overflow", strconv.Itoa(int(^uint16(0)) + 1), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := strToUint16(tt.s)
			if got != tt.want {
				t.Errorf("strToUint16() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.ok {
				t.Errorf("strToUint16() got1 = %v, want %v", got1, tt.ok)
			}
		})
	}
}

func TestArbitrary(t *testing.T) {
	a := NewArbitrary()
	mustLoad := func(s string) {
		if err := a.LoadFromText(s); err != nil {
			t.Fatal(err)
		}
	}

	mustLoad("one.test IN A ANSWER one.test. IN A 192.0.1.1")
	mustLoad("one.test IN A ANSWER one.test. IN A 192.0.1.2")
	mustLoad("one.test IN AAAA ANSWER one.test. IN AAAA ::1")
	q := dns.Question{
		Name:   "one.test.",
		Qtype:  dns.TypeA,
		Qclass: dns.ClassINET,
	}
	rrs := a.Lookup(q)
	if len(rrs) != 2 {
		t.Fatal("invalid rrs length")
	}
	if !rrs[0].RR.(*dns.A).A.Equal(net.ParseIP("192.0.1.1")) {
		t.Fatal("invalid rrs")
	}
	if !rrs[1].RR.(*dns.A).A.Equal(net.ParseIP("192.0.1.2")) {
		t.Fatal("invalid rrs")
	}

	q = dns.Question{
		Name:   "one.test.",
		Qtype:  dns.TypeAAAA,
		Qclass: dns.ClassINET,
	}
	rrs = a.Lookup(q)
	if len(rrs) != 1 {
		t.Fatal("invalid rrs length")
	}
	if !rrs[0].RR.(*dns.AAAA).AAAA.Equal(net.ParseIP("::1")) {
		t.Fatal("invalid rrs")
	}
}
