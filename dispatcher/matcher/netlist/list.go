//     Copyright (C) 2020, IrineSistiana
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
package netlist

import (
	"net"
	"sort"
)

//List is a list of Nets. All Nets will be in ipv6 format, even it's
//ipv4 addr. Cause we use bin search.
type List struct {
	elems  []Net
	sorted bool
}

//NewNetList returns a NetList, list can not be nil.
func NewNetList() *List {
	return &List{
		elems: make([]Net, 0),
	}
}

//Append appends new Nets to the list.
//This modified list, call Sort() before call next Contains()
func (list *List) Append(newNet ...Net) {
	list.elems = append(list.elems, newNet...)
	list.sorted = false
}

// Merge merges srcList with list
// This modified list, call Sort() before call next Contains()
func (list *List) Merge(srcList *List) {
	list.elems = append(list.elems, srcList.elems...)
}

//Sort sorts the list, this must be called everytime after
//list was modified.
func (list *List) Sort() {
	if list.sorted {
		return
	}

	sort.Sort(list)

	result := list.elems[:0]
	lastValid := 0
	for i := range list.elems {
		if i == 0 { // first elem
			result = append(result, list.elems[i])
			continue
		}

		if !list.elems[lastValid].Contains(list.elems[i].ip) {
			result = append(result, list.elems[i])
			lastValid = i
		}
	}

	list.elems = result
	list.sorted = true
}

//implement sort Interface
func (list *List) Len() int {
	return len(list.elems)
}

func (list *List) Less(i, j int) bool {
	return smallOrEqual(list.elems[i].ip, list.elems[j].ip)
}

func (list *List) Swap(i, j int) {
	list.elems[i], list.elems[j] = list.elems[j], list.elems[i]
}

func (list *List) Match(ip net.IP) bool {
	return list.Contains(ip)
}

//Contains reports whether the list includes given ip.
//list must be sorted, or Contains will panic.
func (list *List) Contains(ip net.IP) bool {
	if ip = ip.To16(); ip == nil {
		return false
	}
	ipv6 := Conv(ip)

	if !list.sorted {
		panic("list is not sorted")
	}

	i, j := 0, len(list.elems)
	for i < j {
		h := int(uint(i+j) >> 1) // avoid overflow when computing h

		if smallOrEqual(list.elems[h].ip, ipv6) {
			i = h + 1
		} else {
			j = h
		}
	}

	if i == 0 {
		return false
	}

	return list.elems[i-1].Contains(ipv6)
}

//smallOrEqual IP1 <= IP2 ?
func smallOrEqual(IP1, IP2 IPv6) bool {
	for k := 0; k < 2; k++ {
		if IP1[k] == IP2[k] {
			continue
		}
		return IP1[k] < IP2[k]
	}
	return true
}
