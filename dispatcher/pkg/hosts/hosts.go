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

package hosts

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/matcher/domain"
	"github.com/miekg/dns"
	"net"
)

type Hosts struct {
	matcher domain.Matcher
}

// NewHosts creates a hosts using m.
// The returned v from m.Match() must be the type of IPs.
//
// e.g.
// m := domain.NewMixMatcher()
// domain.BatchLoadMatcher(m, []string{"localhost 127.0.0.1"}, ParseIP)
func NewHosts(m domain.Matcher) *Hosts {
	return &Hosts{
		matcher: m,
	}
}

func NewHostsFromEntries(entries []string) (*Hosts, error) {
	m := domain.NewMixMatcher()
	m.SetPattenTypeMap(domain.MixMatcherStrToPatternTypeDefaultFull)
	if err := domain.BatchLoadMatcher(m, entries, ParseIP); err != nil {
		return nil, err
	}
	return &Hosts{
		matcher: m,
	}, nil
}

func NewHostsFromFiles(files []string) (*Hosts, error) {
	m := domain.NewMixMatcher()
	m.SetPattenTypeMap(domain.MixMatcherStrToPatternTypeDefaultFull)
	if err := domain.BatchLoadMatcherFromFiles(m, files, ParseIP); err != nil {
		return nil, err
	}
	return &Hosts{
		matcher: m,
	}, nil
}

func (h *Hosts) Lookup(fqdn string) (ipv4, ipv6 []net.IP) {
	v, ok := h.matcher.Match(fqdn)
	if !ok {
		return nil, nil // no such host
	}
	ips := v.(*IPs)
	return ips.IPv4, ips.IPv6
}

func (h *Hosts) LookupMsg(m *dns.Msg) *dns.Msg {
	if len(m.Question) != 1 {
		return nil
	}
	q := m.Question[0]
	typ := q.Qtype
	fqdn := q.Name
	if q.Qclass != dns.ClassINET || (typ != dns.TypeA && typ != dns.TypeAAAA) {
		return nil
	}

	ipv4, ipv6 := h.Lookup(fqdn)
	if len(ipv4)+len(ipv6) == 0 {
		return nil // no such host
	}

	r := new(dns.Msg)
	r.SetReply(m)
	switch {
	case typ == dns.TypeA && len(ipv4) > 0:
		for _, ip := range ipv4 {
			ipCopy := make(net.IP, len(ip))
			copy(ipCopy, ip)
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   fqdn,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    3600,
				},
				A: ipCopy,
			}
			r.Answer = append(r.Answer, rr)
		}
	case typ == dns.TypeAAAA && len(ipv6) > 0:
		for _, ip := range ipv6 {
			ipCopy := make(net.IP, len(ip))
			copy(ipCopy, ip)
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   fqdn,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    3600,
				},
				AAAA: ipCopy,
			}
			r.Answer = append(r.Answer, rr)
		}
	}
	return r
}

type IPs struct {
	IPv4 []net.IP
	IPv6 []net.IP
}

func (r *IPs) Append(v interface{}) {
	n, ok := v.(*IPs)
	if !ok {
		return
	}
	r.IPv4 = append(r.IPv4, n.IPv4...)
	r.IPv6 = append(r.IPv6, n.IPv6...)
}

func ParseIP(s []string) (v interface{}, accept bool, err error) {
	if len(s) == 0 {
		return nil, false, nil
	}

	record := new(IPs)
	for _, ipStr := range s {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, false, fmt.Errorf("invalid ip addr %s", ipStr)
		}

		if ipv4 := ip.To4(); ipv4 != nil { // is ipv4
			record.IPv4 = append(record.IPv4, ipv4)
		} else if ipv6 := ip.To16(); ipv6 != nil { // is ipv6
			record.IPv6 = append(record.IPv6, ipv6)
		} else { // invalid
			return nil, false, fmt.Errorf("%s is not an ipv4 or ipv6 addr", ipStr)
		}
	}
	return record, true, nil
}
