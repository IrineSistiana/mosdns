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

package geoip

import (
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"google.golang.org/protobuf/encoding/protowire"
)

const PluginType = "geoip"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

func Init(bp *coremain.BP, args any) (any, error) {
	return NewGeoIP(bp, args.(*Args))
}

type Args struct {
	File string `yaml:"file"`
}

var _ data_provider.GeoIPMatcherProvider = (*GeoIP)(nil)

type GeoIP struct {
	matchers map[string]netlist.Matcher
}

func (g *GeoIP) GetGeoIPMatcher(code string) (netlist.Matcher, bool) {
	m, ok := g.matchers[strings.ToLower(code)]
	return m, ok
}

func NewGeoIP(bp *coremain.BP, args *Args) (*GeoIP, error) {
	matchers, err := LoadGeoIP(args.File)
	if err != nil {
		return nil, err
	}
	return &GeoIP{matchers: matchers}, nil
}

type notMatcher struct {
	m netlist.Matcher
}

func (m notMatcher) Match(addr netip.Addr) bool {
	return !m.m.Match(addr)
}

func LoadGeoIP(file string) (map[string]netlist.Matcher, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return DecodeGeoIPList(b)
}

func DecodeGeoIPList(b []byte) (map[string]netlist.Matcher, error) {
	matchers := make(map[string]netlist.Matcher)
	if err := forEachField(b, func(num protowire.Number, typ protowire.Type, value []byte) error {
		if num != 1 || typ != protowire.BytesType {
			return nil
		}
		code, matcher, err := decodeGeoIP(value)
		if err != nil {
			return err
		}
		if len(code) != 0 {
			matchers[strings.ToLower(code)] = matcher
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return matchers, nil
}

func decodeGeoIP(b []byte) (string, netlist.Matcher, error) {
	var code string
	var inverse bool
	l := netlist.NewList()
	if err := forEachField(b, func(num protowire.Number, typ protowire.Type, value []byte) error {
		switch num {
		case 1:
			if typ == protowire.BytesType {
				code = string(value)
			}
		case 2:
			if typ != protowire.BytesType {
				return nil
			}
			prefix, err := decodeCIDR(value)
			if err != nil {
				return err
			}
			l.Append(prefix)
		case 3:
			if typ != protowire.VarintType {
				return nil
			}
			v, n := protowire.ConsumeVarint(value)
			if n < 0 {
				return protowire.ParseError(n)
			}
			inverse = v != 0
		case 5:
			if typ == protowire.BytesType && len(code) == 0 {
				code = string(value)
			}
		}
		return nil
	}); err != nil {
		return "", nil, err
	}

	l.Sort()
	if inverse {
		return code, notMatcher{m: l}, nil
	}
	return code, l, nil
}

func decodeCIDR(b []byte) (netip.Prefix, error) {
	var ip []byte
	var bits int
	if err := forEachField(b, func(num protowire.Number, typ protowire.Type, value []byte) error {
		switch num {
		case 1:
			if typ == protowire.BytesType {
				ip = value
			}
		case 2:
			if typ != protowire.VarintType {
				return nil
			}
			v, n := protowire.ConsumeVarint(value)
			if n < 0 {
				return protowire.ParseError(n)
			}
			bits = int(v)
		}
		return nil
	}); err != nil {
		return netip.Prefix{}, err
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Prefix{}, fmt.Errorf("invalid CIDR IP %x", ip)
	}
	prefix := netip.PrefixFrom(addr, bits)
	if !prefix.IsValid() {
		return netip.Prefix{}, fmt.Errorf("invalid CIDR prefix %s/%d", addr, bits)
	}
	return prefix.Masked(), nil
}

func forEachField(b []byte, f func(num protowire.Number, typ protowire.Type, value []byte) error) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]

		vn := protowire.ConsumeFieldValue(num, typ, b)
		if vn < 0 {
			return protowire.ParseError(vn)
		}
		value := b[:vn]
		if typ == protowire.BytesType {
			var n int
			value, n = protowire.ConsumeBytes(value)
			if n < 0 {
				return protowire.ParseError(n)
			}
		}
		if err := f(num, typ, value); err != nil {
			return err
		}
		b = b[vn:]
	}
	return nil
}
