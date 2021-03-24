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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"net"
	"strconv"
)

const (
	maxUint64 = ^uint64(0)
)

var (
	ErrInvalidIP = errors.New("invalid ip")
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
func NewNet(ipv6 IPv6, mask int) *Net {
	n := new(Net)
	n.ip = ipv6
	n.mask = cidrMask(mask)
	for i := 0; i < 2; i++ {
		n.ip[i] &= n.mask[i]
	}
	return n
}

//Contains reports whether the net includes the ip.
func (n *Net) Contains(ip IPv6) bool {
	for i := 0; i < 2; i++ {
		if ip[i]&n.mask[i] == n.ip[i] {
			continue
		}
		return false
	}
	return true
}

var v4InV6Prefix uint64 = 0xffff << 32

//Conv converts ip to type IPv6.
//ip should be an ipv4/6 address (with length 4 or 16)
//Conv will return ErrInvalidIP if ip has an invalid length.
func Conv(ip net.IP) (IPv6, error) {
	switch len(ip) {
	case 16:
		ipv6 := IPv6{}
		for i := 0; i < 2; i++ {
			s := i * 8
			ipv6[i] = binary.BigEndian.Uint64(ip[s : s+8])
		}
		return ipv6, nil
	case 4:
		return IPv6{0, uint64(binary.BigEndian.Uint32(ip)) + v4InV6Prefix}, nil
	default:
		return IPv6{}, ErrInvalidIP
	}
}

type IPVersion uint8

const (
	Version4 IPVersion = iota
	Version6
)

func ParseIP(s string) (IPv6, IPVersion, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return IPv6{}, 0, ErrInvalidIP
	}

	ipv6, err := Conv(ip)
	if err != nil {
		return IPv6{}, 0, err
	}

	var v IPVersion
	if ip.To4() != nil {
		v = Version4
	} else {
		v = Version6
	}
	return ipv6, v, nil
}

//ParseCIDR parses s as a CIDR notation IP address and prefix length.
//As defined in RFC 4632 and RFC 4291.
func ParseCIDR(s string) (*Net, error) {
	ipStr, maskStr, ok := utils.SplitString2(s, "/")
	if ok { //has "/"
		//ip
		ipv6, version, err := ParseIP(ipStr)
		if err != nil {
			return nil, err
		}
		//mask
		maskLen, err := strconv.ParseUint(maskStr, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("invalid cidr mask %s", s)
		}

		//if string is a ipv4 addr, add 96
		if version != Version6 {
			maskLen = maskLen + 96
		}

		if maskLen > 128 {
			return nil, fmt.Errorf("cidr mask %s overflow", s)
		}

		return NewNet(ipv6, int(maskLen)), nil
	}

	ipv6, _, err := ParseIP(s)
	if err != nil {
		return nil, err
	}
	return NewNet(ipv6, 128), nil
}

func (ip IPv6) ToNetIP() net.IP {
	nip := make(net.IP, 16)
	uint64ToBytes(ip, nip)
	return nip
}

func (m mask) toNetMask() net.IPMask {
	nMask := make(net.IPMask, 16)
	uint64ToBytes(m, nMask)
	return nMask
}

func uint64ToBytes(in [2]uint64, out []byte) {
	if len(out) < 16 {
		panic("uint64ToBytes: invalid out length")
	}
	binary.BigEndian.PutUint64(out[:8], in[0])
	binary.BigEndian.PutUint64(out[8:], in[1])
}

func (n *Net) ToNetIPNet() *net.IPNet {
	nn := new(net.IPNet)
	nn.IP = n.ip.ToNetIP()
	nn.Mask = n.mask.toNetMask()
	return nn
}

func (n *Net) String() string {
	return n.ToNetIPNet().String()
}

func cidrMask(n int) (m mask) {
	m[0] = ^(maxUint64 >> n)
	if n > 64 {
		m[1] = ^(maxUint64 >> (n - 64))
	}
	return m
}
