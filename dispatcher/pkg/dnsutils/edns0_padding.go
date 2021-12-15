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

package dnsutils

import "github.com/miekg/dns"

// PadToMinimum pads m to the minimum length.
// If the length of m is larger than minLen, PadToMinimum won't do anything.
// upgraded indicates the m was upgraded to an EDNS0 msg.
// newPadding indicates the Padding option is new to m.
func PadToMinimum(m *dns.Msg, minLen int) (upgraded, newPadding bool) {
	l := m.Len()
	if l >= minLen {
		return false, false
	}

	opt := m.IsEdns0()
	if opt != nil {
		if edns0 := GetEDNS0Option(opt, dns.EDNS0PADDING); edns0 != nil { // q is padded.
			pd := edns0.(*dns.EDNS0_PADDING)
			paddingLen := minLen - l + len(pd.Padding)
			if paddingLen < 0 {
				return false, false
			}
			pd.Padding = make([]byte, paddingLen)
			return false, false
		}
		paddingLen := minLen - 4 - l // a Padding option has a 4 bytes header.
		if paddingLen < 0 {
			return false, false
		}
		opt.Option = append(opt.Option, &dns.EDNS0_PADDING{Padding: make([]byte, paddingLen)})
		return false, true
	}

	paddingLen := minLen - 15 - l // 4 bytes padding header + 11 bytes EDNS0 header.
	if paddingLen < 0 {
		return false, false
	}
	opt = UpgradeEDNS0(m)
	opt.Option = append(opt.Option, &dns.EDNS0_PADDING{Padding: make([]byte, paddingLen)})
	return true, true
}
