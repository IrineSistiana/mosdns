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
	"io"
	"os"
	"strings"

	"github.com/miekg/dns"
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
			Name:   strings.ToLower(h.Name),
			Qtype:  h.Rrtype,
			Qclass: h.Class,
		}
		m.m[q] = append(m.m[q], rr)
	}
	return parser.Err()
}

func (m *Matcher) Search(q dns.Question) []dns.RR {
	q.Name = strings.ToLower(q.Name)
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
