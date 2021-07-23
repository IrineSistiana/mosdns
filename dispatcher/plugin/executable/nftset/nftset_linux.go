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

package nftset

import (
	"net"
	"runtime"

	"github.com/google/nftables"
	"github.com/vishvananda/netns"
)

type Entry struct {
	TableName string
	SetName   string
	IP        net.IP
	Mask      uint8
	IsNET6    bool
}

func AddCIDR(e *Entry) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ns, err := netns.Get()
	if err != nil {
		return err
	}
	conn := &nftables.Conn{NetNS: int(ns)}
	table := &nftables.Table{
		Name:   e.TableName,
		Family: nftables.TableFamilyINet,
	}
	set, err := conn.GetSetByName(table, e.SetName)
	if err != nil {
		return err
	}
	var elm nftables.SetElement
	if !e.IsNET6 {
		elm = nftables.SetElement{
			Key: []byte(e.IP.To4()),
		}
	} else {
		elm = nftables.SetElement{
			Key: []byte(e.IP.To16()),
		}
	}
	err = conn.SetAddElements(set, []nftables.SetElement{elm})
	if err != nil {
		return err
	}
	err = conn.Flush()
	return err
}
