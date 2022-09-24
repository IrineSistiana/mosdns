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

package reverselookup

import (
	"errors"
	"net/netip"
	"strings"
)

var errNotPTRDomain = errors.New("domain does not has a ptr suffix")

func parsePTRName(fqdn string) (netip.Addr, error) {
	switch {
	case strings.HasSuffix(fqdn, IP4arpa):
		s := strings.TrimSuffix(fqdn, IP4arpa)
		return reverse4(s)
	case strings.HasSuffix(fqdn, IP6arpa):
		s := strings.TrimSuffix(fqdn, IP6arpa)
		return reverse6(s)
	default:
		return netip.Addr{}, errNotPTRDomain
	}
}

func reverse4(s string) (netip.Addr, error) {
	b := new(strings.Builder)
	b.Grow(15)
	for offset := len(s); offset > 0; {
		l := strings.LastIndexByte(s[:offset], '.')
		b.WriteString(s[l+1 : offset])
		if l != -1 {
			b.WriteByte('.')
		}
		offset = l
	}
	return netip.ParseAddr(b.String())
}

func reverse6(s string) (netip.Addr, error) {
	b := new(strings.Builder)
	b.Grow(63)
	writen := 0
	for i := 0; i < len(s); i++ {
		r := len(s) - 1 - i
		if s[r] == '.' {
			continue
		}
		b.WriteByte(s[r])
		writen++
		if writen != 0 && writen != 32 && writen%4 == 0 {
			b.WriteByte(':')
		}
	}
	return netip.ParseAddr(b.String())
}

const (
	// IP4arpa is the reverse tree suffix for v4 IP addresses.
	IP4arpa = ".in-addr.arpa."
	// IP6arpa is the reverse tree suffix for v6 IP addresses.
	IP6arpa = ".ip6.arpa."
)
