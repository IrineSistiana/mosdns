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

package ecs

import (
	"github.com/miekg/dns"
	"net"
)

func getMsgECS(m *dns.Msg) (e *dns.EDNS0_SUBNET) {
	opt := m.IsEdns0()
	if opt == nil { // no opt, no ecs
		return nil
	}
	// find ecs in opt
	for o := range opt.Option {
		if opt.Option[o].Option() == dns.EDNS0SUBNET {
			return opt.Option[o].(*dns.EDNS0_SUBNET)
		}
	}
	return nil
}

func removeECS(m *dns.Msg) (removedECS *dns.EDNS0_SUBNET) {
	opt := m.IsEdns0()
	if opt == nil { // no opt, no ecs
		return nil
	}

	for i := range opt.Option {
		if opt.Option[i].Option() == dns.EDNS0SUBNET {
			removedECS = opt.Option[i].(*dns.EDNS0_SUBNET)
			opt.Option = append(opt.Option[:i], opt.Option[i+1:]...)
			return
		}
	}
	return nil
}

func setECS(m *dns.Msg, ecs *dns.EDNS0_SUBNET) *dns.Msg {
	opt := m.IsEdns0()
	if opt == nil { // no opt, we need a new opt
		o := new(dns.OPT)
		o.SetUDPSize(dns.MinMsgSize)
		o.Hdr.Name = "."
		o.Hdr.Rrtype = dns.TypeOPT
		o.Option = []dns.EDNS0{ecs}
		m.Extra = append(m.Extra, o)
		return m
	}

	// if m has a opt, search ecs section
	for o := range opt.Option {
		if opt.Option[o].Option() == dns.EDNS0SUBNET { // overwrite
			opt.Option[o] = ecs
			return m
		}
	}

	// no ecs section, append it
	opt.Option = append(opt.Option, ecs)
	return m
}

func newEDNS0Subnet(ip net.IP, mask uint8, v6 bool) *dns.EDNS0_SUBNET {
	edns0Subnet := new(dns.EDNS0_SUBNET)
	// edns family: https://www.iana.org/assignments/address-family-numbers/address-family-numbers.xhtml
	// ipv4 = 1
	// ipv6 = 2
	if !v6 { // ipv4
		edns0Subnet.Family = 1
	} else { // ipv6
		edns0Subnet.Family = 2
	}

	edns0Subnet.SourceNetmask = mask
	edns0Subnet.Code = dns.EDNS0SUBNET
	edns0Subnet.Address = ip

	// SCOPE PREFIX-LENGTH, an unsigned octet representing the leftmost
	// number of significant bits of ADDRESS that the response covers.
	// In queries, it MUST be set to 0.
	// https://tools.ietf.org/html/rfc7871
	edns0Subnet.SourceScope = 0
	return edns0Subnet
}
