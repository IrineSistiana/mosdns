//go:build linux
// +build linux

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

package nftset_utils

import (
	"fmt"
	"github.com/google/nftables"
	"net"
)

type SetIPElem struct {
	// An ipv4 IP must be a 4-byte address.
	// An ipv6 IP must be a 16-byte address.
	IP   net.IP
	Mask int
}

func (s *SetIPElem) String() string {
	return fmt.Sprintf("%s/%d", s.IP, s.Mask)
}

// NtSetHandler can add SetIPElem to corresponding set.
// The table that contains this set must be an inet family table.
// If the set has a 'interval' flag, the SetIPElem.Mask will be
// applied.
type NtSetHandler struct {
	tableFamily nftables.TableFamily
	tableName   string
	setName     string
}

// NewNtSetHandler inits NtSetHandler.
func NewNtSetHandler(family nftables.TableFamily, tableName, setName string) *NtSetHandler {
	return &NtSetHandler{tableFamily: family, tableName: tableName, setName: setName}
}

func (h *NtSetHandler) getSet() (*nftables.Set, error) {
	conn := &nftables.Conn{NetNS: 0} // use default value from github.com/mdlayher/netlink
	table := &nftables.Table{
		Name:   h.tableName,
		Family: h.tableFamily,
	}
	return conn.GetSetByName(table, h.setName)
}

// AddElems adds SetIPElem(s) to set in a single batch.
func (h *NtSetHandler) AddElems(es []*SetIPElem) error {
	set, err := h.getSet()
	if err != nil {
		return fmt.Errorf("failed to get set, %w", err)
	}

	var elems []nftables.SetElement
	if set.Interval {
		elems = make([]nftables.SetElement, 0, 2*len(es))
	} else {
		elems = make([]nftables.SetElement, 0, len(es))
	}

	for _, e := range es {
		if set.Interval {
			mask := net.CIDRMask(e.Mask, len(e.IP)*8)
			start := e.IP.Mask(mask)
			end := broadcastAddr(&net.IPNet{
				IP:   start,
				Mask: mask,
			})
			elems = append(elems, nftables.SetElement{Key: start, IntervalEnd: false}, nftables.SetElement{Key: nextIP(end), IntervalEnd: true})
		} else {
			elems = append(elems, nftables.SetElement{Key: e.IP})
		}
	}

	conn := &nftables.Conn{NetNS: 0}
	err = conn.SetAddElements(set, elems)
	if err != nil {
		return err
	}
	return conn.Flush()
}
