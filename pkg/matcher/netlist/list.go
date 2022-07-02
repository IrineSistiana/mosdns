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

package netlist

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
)

var (
	ErrNotSorted   = errors.New("list is not sorted")
	ErrInvalidAddr = errors.New("addr is invalid")
)

// List is a list of netip.Prefix. It stores all netip.Prefix in one single slice
// and use binary search.
// It is suitable for large static cidr search.
type List struct {
	// stores valid and masked netip.Prefix(s)
	e      []netip.Prefix
	sorted bool
}

// NewList returns a *List.
func NewList() *List {
	return &List{
		e: make([]netip.Prefix, 0),
	}
}

func mustValid(l []netip.Prefix) {
	for i, prefix := range l {
		if !prefix.IsValid() {
			panic(fmt.Sprintf("invalid prefix at #%d", i))
		}
	}
}

// Append appends new netip.Prefix(s) to the list.
// This modified the list. Caller must call List.Sort() before calling List.Contains()
func (list *List) Append(newNet ...netip.Prefix) {
	for i, n := range newNet {
		addr := to6(n.Addr())
		bits := n.Bits()
		if n.Addr().Is4() {
			bits += 96
		}
		newNet[i] = netip.PrefixFrom(addr, bits).Masked()
	}
	mustValid(newNet)
	list.e = append(list.e, newNet...)
	list.sorted = false
}

// Sort sorts the list, this must be called after
// list being modified and before calling List.Contains().
func (list *List) Sort() {
	if list.sorted {
		return
	}

	sort.Sort(list)
	out := make([]netip.Prefix, 0)
	for i, n := range list.e {
		if i == 0 {
			out = append(out, n)
		} else {
			lv := &out[len(out)-1]
			switch {
			case n.Addr() == lv.Addr():
				if n.Bits() < lv.Bits() {
					*lv = n
				}
			case !lv.Contains(n.Addr()):
				out = append(out, n)
			}
		}

	}

	list.e = out
	list.sorted = true
}

// Len implements sort Interface.
func (list *List) Len() int {
	return len(list.e)
}

// Less implements sort Interface.
func (list *List) Less(i, j int) bool {
	return list.e[i].Addr().Less(list.e[j].Addr())
}

// Swap implements sort Interface.
func (list *List) Swap(i, j int) {
	list.e[i], list.e[j] = list.e[j], list.e[i]
}

func (list *List) Match(ip net.IP) (bool, error) {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false, fmt.Errorf("invalid ip %s", ip)
	}
	return list.Contains(addr)
}

// Contains reports whether the list includes the given netip.Addr.
func (list *List) Contains(addr netip.Addr) (bool, error) {
	if !list.sorted {
		return false, ErrNotSorted
	}
	if !addr.IsValid() {
		return false, ErrInvalidAddr
	}

	addr = to6(addr)

	i, j := 0, len(list.e)
	for i < j {
		h := int(uint(i+j) >> 1) // avoid overflow when computing h
		if list.e[h].Addr().Compare(addr) <= 0 {
			i = h + 1
		} else {
			j = h
		}
	}

	if i == 0 {
		return false, nil
	}

	return list.e[i-1].Contains(addr), nil
}

func to6(addr netip.Addr) netip.Addr {
	if addr.Is6() {
		return addr
	}
	return netip.AddrFrom16(addr.As16())
}
