// +build linux

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

package ipset

import (
	"fmt"
	"github.com/miekg/dns"
)

func (p *ipsetPlugin) addIPSet(r *dns.Msg) error {
	for i := range r.Answer {
		entry := new(Entry)

		switch rr := r.Answer[i].(type) {
		case *dns.A:
			entry.IP = rr.A
			entry.SetName = p.setName4
			entry.Mask = p.mask4
			entry.IsNET6 = false
		case *dns.AAAA:
			entry.IP = rr.AAAA
			entry.SetName = p.setName6
			entry.Mask = p.mask6
			entry.IsNET6 = true
		default:
			continue
		}

		if len(entry.SetName) == 0 {
			continue
		}

		err := AddCIDR(entry)
		if err != nil {
			return fmt.Errorf("failed to add ip %s to set %s: %w", entry.IP, entry.SetName, err)
		}
	}

	return nil
}
