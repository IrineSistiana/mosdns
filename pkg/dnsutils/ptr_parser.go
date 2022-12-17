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

package dnsutils

import (
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

var errNotPTRDomain = errors.New("domain does not has a ptr suffix")

const (
	IP4arpa = ".in-addr.arpa."
	IP6arpa = ".ip6.arpa."
)

// ParsePTRQName returns the ip that a PTR query name contains.
func ParsePTRQName(fqdn string) (netip.Addr, error) {
	switch {
	case strings.HasSuffix(fqdn, IP4arpa):
		return reverse4(fqdn[:len(fqdn)-len(IP4arpa)])
	case strings.HasSuffix(fqdn, IP6arpa):
		return reverse6(fqdn[:len(fqdn)-len(IP6arpa)])
	default:
		return netip.Addr{}, errNotPTRDomain
	}
}

func reverse4(s string) (netip.Addr, error) {
	var buf [4]byte
	l := 0
	for offset := len(s); offset > 0 && l < len(buf); l++ {
		var label string
		label, offset = prevLabel(s, offset)
		n, err := strconv.ParseUint(label, 10, 8)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("invaild bit, %w", err)
		}
		buf[l] = byte(n)
	}
	if l < len(buf) {
		return netip.Addr{}, fmt.Errorf("expact at least 3 labels, got %d", l)
	}
	return netip.AddrFrom4(buf), nil
}

func reverse6(s string) (netip.Addr, error) {
	var buf [16]byte
	var val byte
	var tail bool
	var l int
	for offset := len(s); offset > 0 && l < len(buf); {
		var label string
		label, offset = prevLabel(s, offset)
		if len(label) != 1 {
			return netip.Addr{}, fmt.Errorf("invalid label %s", label)
		}
		b := label[0]
		n, ok := hex2byte(b)
		if !ok {
			return netip.Addr{}, fmt.Errorf("invaild bit %d", b)
		}
		if tail {
			buf[l] = val<<4 + n
			l++
			tail = false
		} else {
			val = n
			tail = true
		}
	}
	if l < len(buf) {
		return netip.Addr{}, fmt.Errorf("expact at least 16 bytes, got %d", l)
	}
	return netip.AddrFrom16(buf), nil
}

func hex2byte(c byte) (byte, bool) {
	lower := func(c byte) byte {
		return c | ('x' - 'X')
	}
	var b byte
	switch {
	case '0' <= c && c <= '9':
		b = c - '0'
	case 'a' <= lower(c) && lower(c) <= 'z':
		b = lower(c) - 'a' + 10
	default:
		return 0, false
	}
	return b, true
}

func prevLabel(s string, offset int) (string, int) {
	for {
		s = s[:offset]
		n := strings.LastIndexByte(s, '.')
		label := s[n+1 : offset]
		if n != -1 && len(label) == 0 {
			offset = n
			continue
		}
		return label, n
	}
}
