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

package netlist

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/data_provider"
	"github.com/IrineSistiana/mosdns/v4/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"google.golang.org/protobuf/proto"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
)

type MatcherGroup struct {
	g []Matcher
}

func (m *MatcherGroup) Len() int {
	s := 0
	for _, l := range m.g {
		s += l.Len()
	}
	return s
}

func (m *MatcherGroup) Match(ip net.IP) (bool, error) {
	for _, list := range m.g {
		ok, err := list.Match(ip)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

type DynamicMatcher struct {
	parseFunc func(in []byte) (*List, error)
	v         atomic.Value
}

func NewDynamicMatcher(parseFunc func(in []byte) (*List, error)) *DynamicMatcher {
	return &DynamicMatcher{parseFunc: parseFunc}
}

func (d *DynamicMatcher) Update(newData []byte) error {
	list, err := d.parseFunc(newData)
	if err != nil {
		return err
	}
	d.v.Store(list)
	return nil
}

func (d *DynamicMatcher) Match(ip net.IP) (bool, error) {
	return d.v.Load().(*List).Match(ip)
}

func (d *DynamicMatcher) Len() int {
	return d.v.Load().(*List).Len()
}

// BatchLoadProvider is a helper func to load multiple files using Load.
func BatchLoadProvider(e []string, dm *data_provider.DataManager) (*MatcherGroup, error) {
	mg := new(MatcherGroup)
	staticMatcher := NewList()
	mg.g = append(mg.g, staticMatcher)
	for _, s := range e {
		if strings.HasPrefix(s, "provider:") {
			providerName := strings.TrimPrefix(s, "provider:")
			providerName, v2suffix, _ := strings.Cut(providerName, ":")
			provider := dm.GetDataProvider(providerName)
			if provider == nil {
				return nil, fmt.Errorf("cannot find provider %s", providerName)
			}
			var parseFunc func(in []byte) (*List, error)
			if len(v2suffix) > 0 {
				parseFunc = func(in []byte) (*List, error) {
					return ParseV2rayIPDat(in, v2suffix)
				}
			} else {
				parseFunc = func(in []byte) (*List, error) {
					l := NewList()
					if err := LoadFromReader(l, bytes.NewReader(in)); err != nil {
						return nil, err
					}
					l.Sort()
					return l, nil
				}
			}
			m := NewDynamicMatcher(parseFunc)
			if err := provider.LoadAndAddListener(m); err != nil {
				return nil, fmt.Errorf("failed to load data from provider %s, %w", providerName, err)
			}
			mg.g = append(mg.g, m)
		} else {
			if err := LoadFromText(staticMatcher, s); err != nil {
				return nil, fmt.Errorf("failed to load data %s, %w", s, err)
			}
		}
	}

	staticMatcher.Sort()
	return mg, nil
}

// Load loads data from entry.
// If entry begin with "ext:", Load loads the file by using LoadFromFile.
// Else it loads the entry as a text pattern by using LoadFromText.
func Load(l *List, ip string) error {
	ip = strings.TrimSpace(ip)
	return LoadFromText(l, ip)
}

// LoadFromReader loads IP list from a reader.
// It might modify the List and causes List unsorted.
func LoadFromReader(l *List, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)

	// count how many lines we have read.
	lineCounter := 0
	for scanner.Scan() {
		lineCounter++
		s := scanner.Text()
		s = strings.TrimSpace(s)
		s = utils.RemoveComment(s, "#")
		s = utils.RemoveComment(s, " ")
		if len(s) == 0 {
			continue
		}
		err := LoadFromText(l, s)
		if err != nil {
			return fmt.Errorf("invalid data at line #%d: %w", lineCounter, err)
		}
	}
	return scanner.Err()
}

// LoadFromText loads an IP from s.
// It might modify the List and causes List unsorted.
func LoadFromText(l *List, s string) error {
	if strings.ContainsRune(s, '/') {
		ipNet, err := netip.ParsePrefix(s)
		if err != nil {
			return err
		}
		l.Append(ipNet)
		return nil
	}

	addr, err := netip.ParseAddr(s)
	if err != nil {
		return err
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	l.Append(netip.PrefixFrom(addr, bits))
	return nil
}

func ParseV2rayIPDat(in []byte, args string) (*List, error) {
	v, err := LoadGeoIPListFromDAT(in)
	if err != nil {
		return nil, err
	}
	return NewV2rayIPDat(v, args)
}

// NewV2rayIPDat builds a List from given v and args.
// The format of args is "tag1,tag2,...".
// Only lists that are matched by given tags will be loaded to List.
func NewV2rayIPDat(v *v2data.GeoIPList, args string) (*List, error) {
	m := make(map[string][]*v2data.CIDR)
	for _, gs := range v.GetEntry() {
		m[strings.ToLower(gs.GetCountryCode())] = gs.GetCidr()
	}

	l := NewList()
	for _, tag := range strings.Split(args, ",") {
		cidrs := m[tag]
		if cidrs == nil {
			return nil, fmt.Errorf("tag %s does not exist", tag)
		}
		if err := LoadFromV2CIDR(l, cidrs); err != nil {
			return nil, fmt.Errorf("failed to parse v2 cidr data, %w", err)
		}

	}
	l.Sort()
	return l, nil
}

// LoadFromV2CIDR loads ip from v2ray CIDR.
// It might modify the List and causes List unsorted.
func LoadFromV2CIDR(l *List, cidr []*v2data.CIDR) error {
	for i, e := range cidr {
		ip, ok := netip.AddrFromSlice(e.Ip)
		if !ok {
			return fmt.Errorf("invalid data ip at index #%d: %s", i, e.Ip)
		}

		prefix := e.Prefix
		if len(e.Ip) == 4 {
			prefix += 96
		}
		l.Append(netip.PrefixFrom(netip.AddrFrom16(ip.As16()), int(e.Prefix)))
	}
	return nil
}

func LoadGeoIPListFromDAT(b []byte) (*v2data.GeoIPList, error) {
	geoIP := new(v2data.GeoIPList)
	if err := proto.Unmarshal(b, geoIP); err != nil {
		return nil, err
	}
	return geoIP, nil
}
