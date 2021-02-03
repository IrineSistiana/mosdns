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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/matcher/v2data"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/golang/protobuf/proto"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"time"
)

var matcherCache = utils.NewCache()

const (
	cacheTTL = time.Second * 30
)

type MatcherGroup struct {
	m []Matcher
}

func (mg *MatcherGroup) Match(ip net.IP) bool {
	for _, m := range mg.m {
		if m.Match(ip) {
			return true
		}
	}
	return false
}

func NewMatcherGroup(m []Matcher) *MatcherGroup {
	return &MatcherGroup{m: m}
}

// BatchLoad is helper func to load multiple files using NewListFromFile.
func BatchLoad(f []string) (m *List, err error) {
	if len(f) == 0 {
		return nil, errors.New("no file to load")
	}

	if len(f) == 1 {
		return NewListFromFile(f[0])
	}

	list := NewList()
	for _, file := range f {
		l, err := NewListFromFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to load ip file %s: %w", file, err)
		}
		list.Merge(l)
	}

	list.Sort()
	return list, nil
}

//NewListFromReader read IP list from a reader. The returned *List is sorted.
func NewListFromReader(reader io.Reader) (*List, error) {
	ipNetList := NewList()
	scanner := bufio.NewScanner(reader)

	//count how many lines we have read.
	lineCounter := 0

	for scanner.Scan() {
		lineCounter++
		s := strings.TrimSpace(utils.BytesToStringUnsafe(scanner.Bytes()))
		s = utils.RemoveComment(s, "#")
		s = utils.RemoveComment(s, " ") // remove other strings, e.g. 192.168.1.1 str1 str2

		if len(s) == 0 {
			continue
		}

		ipNet, err := ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR format %s in line %d", s, lineCounter)
		}

		ipNetList.Append(ipNet)
	}

	ipNetList.Sort()
	return ipNetList, nil
}

// NewListFromFile loads ip from a text file or a geoip file.
// If file contains a ':' and has format like 'geoip:cn', it will be read as a geoip file.
// The returned *List is already been sorted.
func NewListFromFile(file string) (*List, error) {
	if strings.Contains(file, ":") {
		tmp := strings.SplitN(file, ":", 2)
		return NewListFromDAT(tmp[0], tmp[1]) // file and tag
	} else {
		return NewListFromTextFile(file)
	}
}

// NewListFromTextFile reads IP list from a text file.
// The returned *List is already been sorted.
func NewListFromTextFile(file string) (*List, error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	return NewListFromReader(bytes.NewBuffer(b))
}

// NewListFromDAT loads ip from v2ray proto file.
// The returned *List is already been sorted.
func NewListFromDAT(file, tag string) (*List, error) {
	geoIP, err := LoadGeoIPFromDAT(file, tag)
	if err != nil {
		return nil, err
	}
	return NewListFromV2CIDR(geoIP.GetCidr())
}

// NewListFromV2CIDR loads ip from v2ray CIDR.
// The returned *List is already been sorted.
func NewListFromV2CIDR(cidr []*v2data.CIDR) (*List, error) {
	l := NewList()
	l.Grow(len(cidr))

	for i, e := range cidr {
		ip6 := net.IP(e.Ip).To16()
		if ip6 == nil {
			return nil, fmt.Errorf("invalid cidr ip at #%d", i)
		}
		ipv6 := Conv(ip6)
		switch len(e.Ip) {
		case 4:
			l.Append(NewNet(ipv6, uint(e.Prefix+96)))
		case 16:
			l.Append(NewNet(ipv6, uint(e.Prefix)))
		default:
			return nil, fmt.Errorf("invalid cidr ip length at #%d", i)
		}
	}

	l.Sort()
	return l, nil
}

func LoadGeoIPFromDAT(file, tag string) (*v2data.GeoIP, error) {
	geoIPList, err := LoadGeoIPListFromDAT(file)
	if err != nil {
		return nil, err
	}

	entry := geoIPList.GetEntry()
	upperTag := strings.ToUpper(tag)
	for i := range entry {
		if strings.ToUpper(entry[i].CountryCode) == upperTag {
			return entry[i], nil
		}
	}

	return nil, fmt.Errorf("can not find tag %s in %s", tag, file)
}

func LoadGeoIPListFromDAT(file string) (*v2data.GeoIPList, error) {
	data, raw, err := matcherCache.LoadFromCacheOrRawDisk(file)
	if err != nil {
		return nil, err
	}
	// load from cache
	if geoIPList, ok := data.(*v2data.GeoIPList); ok {
		return geoIPList, nil
	}

	// load from disk
	geoIPList := new(v2data.GeoIPList)
	if err := proto.Unmarshal(raw, geoIPList); err != nil {
		return nil, err
	}

	// cache the file
	matcherCache.Put(file, geoIPList, cacheTTL)
	return geoIPList, nil
}
