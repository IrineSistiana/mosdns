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

package v2data

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/load_cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/netlist"
	"google.golang.org/protobuf/proto"
	"os"
	"strings"
	"time"
)

// LoadNetListFromDAT loads ip from v2ray proto file.
// It might modify the List and causes List unsorted.
func LoadNetListFromDAT(l *netlist.List, file, tag string) error {
	geoIP, err := LoadGeoIPFromDAT(file, tag)
	if err != nil {
		return err
	}
	return LoadFromV2CIDR(l, geoIP.GetCidr())
}

// LoadFromV2CIDR loads ip from v2ray CIDR.
// It might modify the List and causes List unsorted.
func LoadFromV2CIDR(l *netlist.List, cidr []*CIDR) error {
	for i, e := range cidr {
		ipv6, err := netlist.Conv(e.Ip)
		if err != nil {
			return fmt.Errorf("invalid data ip at index #%d, %w", i, err)
		}
		switch len(e.Ip) {
		case 4:
			l.Append(netlist.NewNet(ipv6, int(e.Prefix+96)))
		case 16:
			l.Append(netlist.NewNet(ipv6, int(e.Prefix)))
		default:
			return fmt.Errorf("invalid cidr ip length at #%d", i)
		}
	}
	return nil
}

func LoadGeoIPFromDAT(file, tag string) (*GeoIP, error) {
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

var geoIPCache = load_cache.GetCache().NewNamespace()

func LoadGeoIPListFromDAT(file string) (*GeoIPList, error) {
	// load from cache
	v, _ := geoIPCache.Get(file)
	if geoIP, ok := v.(*GeoIPList); ok {
		return geoIP, nil
	}

	// load from disk
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	geoIP := new(GeoIPList)
	if err := proto.Unmarshal(raw, geoIP); err != nil {
		return nil, err
	}

	// cache the file
	geoIPCache.Store(file, geoIP)
	time.AfterFunc(time.Second*15, func() { // remove it after 15s
		geoIPCache.Remove(file)
	})
	return geoIP, nil
}
