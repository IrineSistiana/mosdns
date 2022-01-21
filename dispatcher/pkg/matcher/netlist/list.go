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
//     along with this program.  If not, see <https:// www.gnu.org/licenses/>.

package netlist

import (
	"errors"
	"net"
	"sort"
)

var ErrNotSorted = errors.New("list is not sorted")

// List is a list of Net. It stores all Net in one single slice
// and use binary search.
// It is suitable for large static cidr search.
type List struct {
	e      []Net
	sorted bool
}

// NewList returns a *List.
func NewList() *List {
	return &List{
		e: make([]Net, 0),
	}
}

// NewListFrom returns a *List using l as its initial contents.
// The new List takes ownership of l, and the caller should not use l after this call.
func NewListFrom(l []Net) *List {
	return &List{
		e: l,
	}
}

// Append appends new Nets to the list.
// This modified the list. Caller must call List.Sort() before calling List.Contains()
func (list *List) Append(newNet ...Net) {
	list.e = append(list.e, newNet...)
	list.sorted = false
}

// Merge merges srcList with list
// This modified the list. Caller must call List.Sort() before calling List.Contains()
func (list *List) Merge(srcList *List) {
	list.e = append(list.e, srcList.e...)
	list.sorted = false
}

// Sort sorts the list, this must be called after
// list being modified and before calling List.Contains().
func (list *List) Sort() {
	if list.sorted {
		return
	}

	sort.Sort(list)

	result := list.e[:0]
	var lastValid *Net
	for i, n := range list.e {
		switch {
		case i == 0:
			result = append(result, n)
			lastValid = &result[len(result)-1]
		case lastValid.ip == n.ip:
			if n.mask < lastValid.mask {
				lastValid.mask = n.mask
			}
		case !lastValid.Contains(n.ip):
			result = append(result, n)
			lastValid = &result[len(result)-1]
		}
	}

	list.e = result
	list.sorted = true
}

// Len implements sort Interface.
func (list *List) Len() int {
	return len(list.e)
}

// Less implements sort Interface.
func (list *List) Less(i, j int) bool {
	return smallOrEqual(list.e[i].ip, list.e[j].ip)
}

// Swap implements sort Interface.
func (list *List) Swap(i, j int) {
	list.e[i], list.e[j] = list.e[j], list.e[i]
}

func (list *List) Match(ip net.IP) (bool, error) {
	ipv6, err := Conv(ip)
	if err != nil {
		return false, err
	}

	return list.Contains(ipv6)
}

// Contains reports whether the list includes the given ipv6.
func (list *List) Contains(ipv6 IPv6) (bool, error) {
	if !list.sorted {
		return false, ErrNotSorted
	}

	i, j := 0, len(list.e)
	for i < j {
		h := int(uint(i+j) >> 1) // avoid overflow when computing h

		if smallOrEqual(list.e[h].ip, ipv6) {
			i = h + 1
		} else {
			j = h
		}
	}

	if i == 0 {
		return false, nil
	}

	return list.e[i-1].Contains(ipv6), nil
}

// smallOrEqual IP1 <= IP2 ?
func smallOrEqual(IP1, IP2 IPv6) bool {
	for k := 0; k < 2; k++ {
		if IP1[k] == IP2[k] {
			continue
		}
		return IP1[k] < IP2[k]
	}
	return true
}
