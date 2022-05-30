package zone_file

import (
	"github.com/miekg/dns"
	"io"
	"os"
)

type Matcher struct {
	m map[dns.Question][]dns.RR
}

func (m *Matcher) LoadFile(s string) error {
	f, err := os.Open(s)
	if err != nil {
		return err
	}
	defer f.Close()

	return m.Load(f)
}

func (m *Matcher) Load(r io.Reader) error {
	if m.m == nil {
		m.m = make(map[dns.Question][]dns.RR)
	}

	parser := dns.NewZoneParser(r, "", "")
	parser.SetDefaultTTL(3600)
	for {
		rr, ok := parser.Next()
		if !ok {
			break
		}
		h := rr.Header()
		q := dns.Question{
			Name:   h.Name,
			Qtype:  h.Rrtype,
			Qclass: h.Class,
		}
		m.m[q] = append(m.m[q], rr)
	}
	return parser.Err()
}

func (m *Matcher) Search(q dns.Question) []dns.RR {
	return m.m[q]
}

func (m *Matcher) Reply(q *dns.Msg) *dns.Msg {
	var r *dns.Msg
	for _, question := range q.Question {
		rr := m.Search(question)
		if rr != nil {
			if r == nil {
				r = new(dns.Msg)
				r.SetReply(q)
			}
			r.Answer = append(r.Answer, rr...)
		}
	}
	return r
}
