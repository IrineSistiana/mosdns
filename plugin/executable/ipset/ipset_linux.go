//go:build linux

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

package ipset

import (
	"net"
	"syscall"

	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
)

const (
	IPSET_ATTR_IPADDR_IPV4 = 1
	IPSET_ATTR_IPADDR_IPV6 = 2
)

type Entry struct {
	SetName string
	IP      net.IP
	Mask    uint8
	IsNET6  bool
}

func AddCIDR(e *Entry) error {
	req := nl.NewNetlinkRequest(nl.IPSET_CMD_ADD|(unix.NFNL_SUBSYS_IPSET<<8), nl.GetIpsetFlags(nl.IPSET_CMD_ADD))

	var nfgenFamily uint8
	if e.IsNET6 {
		nfgenFamily = uint8(unix.AF_INET6)
	} else {
		nfgenFamily = uint8(unix.AF_INET)
	}
	req.AddData(
		&nl.Nfgenmsg{
			NfgenFamily: nfgenFamily,
			Version:     nl.NFNETLINK_V0,
			ResId:       0,
		},
	)

	req.AddData(nl.NewRtAttr(nl.IPSET_ATTR_PROTOCOL, nl.Uint8Attr(nl.IPSET_PROTOCOL)))
	req.AddData(nl.NewRtAttr(nl.IPSET_ATTR_SETNAME, nl.ZeroTerminated(e.SetName)))
	data := nl.NewRtAttr(nl.IPSET_ATTR_DATA|int(nl.NLA_F_NESTED), nil)

	// set ip
	addr := nl.NewRtAttr(nl.IPSET_ATTR_IP|int(nl.NLA_F_NESTED), nil)
	if e.IsNET6 {
		addr.AddRtAttr(IPSET_ATTR_IPADDR_IPV6|int(nl.NLA_F_NET_BYTEORDER), e.IP)
	} else {
		addr.AddRtAttr(IPSET_ATTR_IPADDR_IPV4|int(nl.NLA_F_NET_BYTEORDER), e.IP)
	}
	data.AddChild(addr)

	// set mask
	data.AddRtAttr(nl.IPSET_ATTR_CIDR, nl.Uint8Attr(e.Mask))

	req.AddData(data)
	_, err := req.Execute(unix.NETLINK_NETFILTER, 0)

	if err != nil {
		if errno := int(err.(syscall.Errno)); errno >= nl.IPSET_ERR_PRIVATE {
			err = nl.IPSetError(uintptr(errno))
		}
	}
	return err
}
