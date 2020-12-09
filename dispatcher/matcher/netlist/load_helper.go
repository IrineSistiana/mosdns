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
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/golang/protobuf/proto"
	"io"
	"strings"
	"time"
	"v2ray.com/core/app/router"
)

var matcherCache = utils.NewCache()

const (
	cacheTTL = time.Second * 30
)

//NewListFromReader read IP list from a reader, if no valid IP addr was found,
//it will return a empty NetList, NOT nil. NetList will be a sorted list.
func NewListFromReader(reader io.Reader, continueOnInvalidString bool) (*List, error) {

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
			if continueOnInvalidString {
				mlog.Entry().Warnf("invalid CIDR format %s in line %d", line, lineCounter)
				continue
			} else {
				return nil, fmt.Errorf("invalid CIDR format %s in line %d", line, lineCounter)
			}
		}

		ipNetList.Append(ipNet)
	}

	ipNetList.Sort()
	return ipNetList, nil
}

// NewIPMatcherFromFile loads a netlist file a list or geoip file.
// if file contains a ':' and has format like 'geoip:cn', file must be a geoip file.
func NewIPMatcherFromFile(file string) (Matcher, error) {
	e, ok := matcherCache.Load(file)
	if ok {
		if m, ok := e.(Matcher); ok {
			return m, nil
		}
	}

	var m Matcher
	var err error
	if strings.Contains(file, ":") {
		tmp := strings.SplitN(file, ":", 2)
		m, err = NewNetListFromDAT(tmp[0], tmp[1]) // file and tag
	} else {
		m, err = NewListFromListFile(file, true)
	}

	if err != nil {
		return nil, err
	}

	matcherCache.Put(file, m, cacheTTL)
	return m, nil
}

// NewListFromFile read IP list from a file, the returned NetList is already been sorted.
func NewListFromListFile(file string, continueOnInvalidString bool) (Matcher, error) {
	data, raw, err := matcherCache.LoadFromCacheOrRawDisk(file)
	if err != nil {
		return nil, err
	}

	// load from cache
	if nl, ok := data.(*List); ok {
		return nl, nil
	}
	// load from disk
	return NewListFromReader(bytes.NewBuffer(raw), continueOnInvalidString)
}

func NewNetListFromDAT(file, tag string) (Matcher, error) {
	cidrList, err := loadV2CIDRListFromDAT(file, tag)
	if err != nil {
		return nil, err
	}

	return NewV2Matcher(cidrList)
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
	for i := range entry {
		if strings.ToUpper(entry[i].CountryCode) == strings.ToUpper(tag) {
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
