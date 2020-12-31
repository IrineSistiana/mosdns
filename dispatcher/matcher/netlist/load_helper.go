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

package netlist

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/golang/protobuf/proto"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"time"
	"v2ray.com/core/app/router"
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

// BatchLoad is helper func to load multiple files using NewIPMatcherFromFile.
func BatchLoad(f []string) (m Matcher, err error) {
	if len(f) == 0 {
		return nil, errors.New("no file to load")
	}

	if len(f) == 1 {
		return NewIPMatcherFromFile(f[0])
	}

	groupMatcher := make([]Matcher, 0)
	for _, file := range f {
		m, err := NewIPMatcherFromFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to load ip file %s: %w", file, err)
		}
		groupMatcher = append(groupMatcher, m)
	}
	return NewMatcherGroup(groupMatcher), nil
}

//NewListFromReader read IP list from a reader, if no valid IP addr was found,
//it will return a empty NetList, NOT nil. NetList will be a sorted list.
func NewListFromReader(reader io.Reader) (*List, error) {

	ipNetList := NewNetList()
	s := bufio.NewScanner(reader)

	//count how many lines we have read.
	lineCounter := 0

	for s.Scan() {
		lineCounter++
		line := strings.TrimSpace(s.Text())

		//ignore lines begin with # and empty lines
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		ipNet, err := ParseCIDR(line)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR format %s in line %d", line, lineCounter)
		}

		ipNetList.Append(ipNet)
	}

	ipNetList.Sort()
	return ipNetList, nil
}

// NewIPMatcherFromFile loads a netlist file a list or geoip file.
// if file contains a ':' and has format like 'geoip:cn', file must be a geoip file.
func NewIPMatcherFromFile(file string) (Matcher, error) {
	var m Matcher
	var err error
	if strings.Contains(file, ":") {
		tmp := strings.SplitN(file, ":", 2)
		m, err = NewNetListFromDAT(tmp[0], tmp[1]) // file and tag
	} else {
		m, err = NewListFromListFile(file)
	}

	if err != nil {
		return nil, err
	}
	return m, nil
}

// NewListFromListFile read IP list from a file, the returned NetList is already been sorted.
func NewListFromListFile(file string) (Matcher, error) {
	// load from cache
	if v, ok := matcherCache.Load(file); ok {
		if nl, ok := v.(*List); ok {
			return nl, nil
		}
	}

	// load from disk
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	m, err := NewListFromReader(bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	matcherCache.Put(file, m, cacheTTL)
	return m, nil
}

func NewNetListFromDAT(file, tag string) (Matcher, error) {
	// load from cache
	if v, ok := matcherCache.Load(file); ok {
		if v2Matcher, ok := v.(*V2Matcher); ok {
			return v2Matcher, nil
		}
	}

	cidrList, err := loadV2CIDRListFromDAT(file, tag)
	if err != nil {
		return nil, err
	}
	v2Matcher, err := NewV2Matcher(cidrList)
	if err != nil {
		return nil, err
	}

	matcherCache.Put(file, v2Matcher, cacheTTL)
	return v2Matcher, nil
}

func loadV2CIDRListFromDAT(file, tag string) ([]*router.CIDR, error) {
	geoIP, err := loadGeoIPFromDAT(file, tag)
	if err != nil {
		return nil, err
	}
	return geoIP.GetCidr(), nil
}

func loadGeoIPFromDAT(file, tag string) (*router.GeoIP, error) {
	geoIPList, err := loadGeoIPListFromDAT(file)
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

func loadGeoIPListFromDAT(file string) (*router.GeoIPList, error) {
	data, raw, err := matcherCache.LoadFromCacheOrRawDisk(file)
	if err != nil {
		return nil, err
	}
	// load from cache
	if geoIPList, ok := data.(*router.GeoIPList); ok {
		return geoIPList, nil
	}

	// load from disk
	geoIPList := new(router.GeoIPList)
	if err := proto.Unmarshal(raw, geoIPList); err != nil {
		return nil, err
	}

	// cache the file
	matcherCache.Put(file, geoIPList, cacheTTL)
	return geoIPList, nil
}
