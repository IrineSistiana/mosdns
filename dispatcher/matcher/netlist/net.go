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

package netlist

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	maxUint64 = ^uint64(0)
)

//IPv6 represents a ipv6 addr
type IPv6 [2]uint64

//mask is ipv6 IP network mask
type mask [2]uint64

//Net represents a ip network
type Net struct {
	ip   IPv6
	mask mask
}

//NewNet returns a new IPNet, mask should be an ipv6 mask,
//which means you should +96 if you have an ipv4 mask.
func NewNet(ipv6 IPv6, mask uint) (n Net) {
	n.ip = ipv6
	n.mask = cidrMask(mask)
	for i := 0; i < 2; i++ {
		n.ip[i] &= n.mask[i]
	}
	return
}

//Contains reports whether the net includes the ip.
func (net Net) Contains(ip IPv6) bool {
	for i := 0; i < 2; i++ {
		if ip[i]&net.mask[i] == net.ip[i] {
			continue
		}
		return false
	}
	return true
}

//Conv converts ip to type IPv6.
//ip must be a valid 16-byte ipv6 address or Conv() will panic
func Conv(ip net.IP) (ipv6 IPv6) {
	if len(ip) != 16 {
		panic("ip is not a 16-byte ipv6")
	}

	for i := 0; i < 2; i++ { //0 to 1
		s := i * 8
		ipv6[i] = binary.BigEndian.Uint64(ip[s : s+8])
	}

	return
}

//ParseCIDR parses s as a CIDR notation IP address and prefix length.
//As defined in RFC 4632 and RFC 4291.
func ParseCIDR(s string) (Net, error) {

	sub := strings.SplitN(s, "/", 2)
	if len(sub) == 2 { //has "/"
		addrStr, maskStr := sub[0], sub[1]

		//ip
		ip := net.ParseIP(addrStr).To16()
		if ip == nil {
			return Net{}, fmt.Errorf("invalid cidr ip string %s", s)
		}
		ipv6 := Conv(ip)

		//mask
		maskLen, err := strconv.ParseUint(maskStr, 10, 0)
		if err != nil {
			return Net{}, fmt.Errorf("invalid cidr mask %s", s)
		}

		//if string is a ipv4 addr, add 96
		if ip.To4() != nil {
			maskLen = maskLen + 96
		}

		if maskLen > 128 {
			return Net{}, fmt.Errorf("cidr mask %s overflow", s)
		}

		return NewNet(ipv6, uint(maskLen)), nil
	}

	ip := net.ParseIP(s).To16()
	if ip == nil {
		return Net{}, fmt.Errorf("invalid cidr ip string %s", s)
	}
	ipv6 := Conv(ip)
	return NewNet(ipv6, 128), nil
}

func cidrMask(n uint) (m mask) {
	for i := uint(0); i < 2; i++ {
		if n != 0 {
			m[i] = ^(maxUint64 >> n)
		} else {
			break
		}

		if n > 64 {
			n = n - 64
		} else {
			break
		}
	}

	return m
}
