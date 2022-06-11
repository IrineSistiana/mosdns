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

import "github.com/miekg/dns"

// UpgradeEDNS0 enables EDNS0 for m and returns it's dns.OPT record.
// m must be a msg without dns.OPT.
func UpgradeEDNS0(m *dns.Msg) *dns.OPT {
	o := new(dns.OPT)
	o.SetUDPSize(dns.MinMsgSize)
	o.Hdr.Name = "."
	o.Hdr.Rrtype = dns.TypeOPT
	m.Extra = append(m.Extra, o)
	return o
}

// RemoveEDNS0 removes the OPT record from m.
func RemoveEDNS0(m *dns.Msg) {
	for i := len(m.Extra) - 1; i >= 0; i-- {
		if m.Extra[i].Header().Rrtype == dns.TypeOPT {
			m.Extra = append(m.Extra[:i], m.Extra[i+1:]...)
			return
		}
	}
	return
}

func RemoveEDNS0Option(opt *dns.OPT, option uint16) {
	for i := range opt.Option {
		if opt.Option[i].Option() == option {
			opt.Option = append(opt.Option[:i], opt.Option[i+1:]...)
			return
		}
	}
	return
}

func GetEDNS0Option(opt *dns.OPT, option uint16) dns.EDNS0 {
	for o := range opt.Option {
		if opt.Option[o].Option() == option {
			return opt.Option[o]
		}
	}
	return nil
}
