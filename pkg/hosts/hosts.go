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

package hosts

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/miekg/dns"
	"net/netip"
	"strings"
)

type Hosts struct {
	matcher domain.Matcher[*IPs]
}

// NewHosts creates a hosts using m.
func NewHosts(m domain.Matcher[*IPs]) *Hosts {
	return &Hosts{
		matcher: m,
	}
}

func (h *Hosts) GetMatcher() domain.Matcher[*IPs] {
	return h.matcher
}

func (h *Hosts) Lookup(fqdn string) (ipv4, ipv6 []netip.Addr) {
	ips, ok := h.matcher.Match(fqdn)
	if !ok {
		return nil, nil // no such host
	}
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
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   fqdn,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    10,
				},
				A: ip.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
	case typ == dns.TypeAAAA && len(ipv6) > 0:
		for _, ip := range ipv6 {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   fqdn,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    10,
				},
				AAAA: ip.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
	}

	// Append fake SOA record for empty reply.
	if len(r.Answer) == 0 {
		r.Ns = []dns.RR{dnsutils.FakeSOA(fqdn)}
	}
	return r
}

type IPs struct {
	IPv4 []netip.Addr
	IPv6 []netip.Addr
}

var _ domain.ParseStringFunc[*IPs] = ParseIPs

func ParseIPs(s string) (string, *IPs, error) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return "", nil, errors.New("empty string")
	}

	pattern := f[0]
	v := new(IPs)
	for _, ipStr := range f[1:] {
		ip, err := netip.ParseAddr(ipStr)
		if err != nil {
			return "", nil, fmt.Errorf("invalid ip addr %s, %w", ipStr, err)
		}

		if ip.Is4() { // is ipv4
			v.IPv4 = append(v.IPv4, ip)
		} else { // is ipv6
			v.IPv6 = append(v.IPv6, ip)
		}
	}

	return pattern, v, nil
}
