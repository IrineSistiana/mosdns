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
	"fmt"
	"github.com/miekg/dns"
)

func (p *nftsetPlugin) addNFTSet(r *dns.Msg) error {
	for i := range r.Answer {
		var entry *Entry

		switch rr := r.Answer[i].(type) {
		case *dns.A:
			if len(p.args.SetName4) == 0 {
				continue
			}
			entry = &Entry{
				SetName: p.args.SetName4,
				IP:      rr.A,
				Mask:    p.args.Mask4,
				IsNET6:  false,
			}
			if p.args.MaxTTL4 > 0 && rr.Hdr.Ttl > p.args.MaxTTL4 {
				rr.Hdr.Ttl = p.args.MaxTTL4
			}
		case *dns.AAAA:
			if len(p.args.SetName6) == 0 {
				continue
			}
			entry = &Entry{
				SetName: p.args.SetName6,
				IP:      rr.AAAA,
				Mask:    p.args.Mask6,
				IsNET6:  true,
			}
			if p.args.MaxTTL6 > 0 && rr.Hdr.Ttl > p.args.MaxTTL6 {
				rr.Hdr.Ttl = p.args.MaxTTL6
			}
		default:
			continue
		}
		entry.TableName = p.args.TableName
		err := AddCIDR(entry)
		if err != nil {
			return fmt.Errorf("failed to add ip %s to set %s: %w", entry.IP, entry.SetName, err)
		}
	}

	return nil
}
