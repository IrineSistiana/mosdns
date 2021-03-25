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
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/load_cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/golang/protobuf/proto"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

var matcherCache = load_cache.GetCache().NewNamespace()

// BatchLoad is a helper func to load multiple files using Load.
// It might modify the List and causes List unsorted.
func BatchLoad(l *List, entries []string) error {
	for _, file := range entries {
		err := Load(l, file)
		if err != nil {
			return fmt.Errorf("failed to load ip file %s: %w", file, err)
		}
	}
	return nil
}

// Load loads data from entry.
// If entry begin with "ext:", Load loads the file by using LoadFromFile.
// Else it loads the entry as a text pattern by using LoadFromText.
func Load(l *List, entry string) error {
	s1, s2, ok := utils.SplitString2(entry, ":")
	if ok && s1 == "ext" {
		return LoadFromFile(l, s2)
	}
	return LoadFromText(l, entry)
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
	ipNet, err := ParseCIDR(s)
	if err != nil {
		return err
	}
	l.Append(ipNet)
	return nil
}

// LoadFromFile loads ip from a text file or a geoip file.
// If file contains a ':' and has format like 'geoip:cn', it will be read as a geoip file.
// It might modify the List and causes List unsorted.
func LoadFromFile(l *List, file string) error {
	if strings.Contains(file, ":") {
		tmp := strings.SplitN(file, ":", 2)
		return LoadFromDAT(l, tmp[0], tmp[1]) // file and tag
	} else {
		return LoadFromTextFile(l, file)
	}
}

// LoadFromTextFile reads IP list from a text file.
// It might modify the List and causes List unsorted.
func LoadFromTextFile(l *List, file string) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	return LoadFromReader(l, bytes.NewBuffer(b))
}

// LoadFromDAT loads ip from v2ray proto file.
// It might modify the List and causes List unsorted.
func LoadFromDAT(l *List, file, tag string) error {
	geoIP, err := LoadGeoIPFromDAT(file, tag)
	if err != nil {
		return err
	}
	return LoadFromV2CIDR(l, geoIP.GetCidr())
}

// LoadFromV2CIDR loads ip from v2ray CIDR.
// It might modify the List and causes List unsorted.
func LoadFromV2CIDR(l *List, cidr []*v2data.CIDR) error {
	for i, e := range cidr {
		ipv6, err := Conv(e.Ip)
		if err != nil {
			return fmt.Errorf("invalid data ip at index #%d, %w", i, err)
		}
		switch len(e.Ip) {
		case 4:
			l.Append(NewNet(ipv6, int(e.Prefix+96)))
		case 16:
			l.Append(NewNet(ipv6, int(e.Prefix)))
		default:
			return fmt.Errorf("invalid cidr ip length at #%d", i)
		}
	}
	return nil
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
	// load from cache
	v, _ := matcherCache.Get(file)
	if geoIP, ok := v.(*v2data.GeoIPList); ok {
		return geoIP, nil
	}

	// load from disk
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	geoIP := new(v2data.GeoIPList)
	if err := proto.Unmarshal(raw, geoIP); err != nil {
		return nil, err
	}

	// cache the file
	matcherCache.Store(file, geoIP)
	time.AfterFunc(time.Second*15, func() { // remove it after 15s
		matcherCache.Remove(file)
	})
	return geoIP, nil
}
