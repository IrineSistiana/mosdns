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
