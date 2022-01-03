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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/load_cache"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/domain"
	"google.golang.org/protobuf/proto"
	"os"
	"strings"
	"time"
)

func LoadMixMatcherFromDAT(m *domain.MixMatcher, file, countryCode string, processAttr domain.ProcessAttrFunc) error {
	geoSite, err := LoadGeoSiteFromDAT(file, countryCode)
	if err != nil {
		return err
	}

	for _, d := range geoSite.GetDomain() {
		attr := make([]string, 0, len(d.Attribute))
		for _, a := range d.Attribute {
			attr = append(attr, a.Key)
		}

		var v interface{}
		if processAttr != nil {
			var accept bool
			var err error
			v, accept, err = processAttr(attr)
			if err != nil {
				return err
			}
			if !accept {
				return nil
			}
		}

		var typ domain.MixMatcherPatternType
		switch d.Type {
		case Domain_Plain:
			typ = domain.MixMatcherPatternTypeKeyword
		case Domain_Regex:
			typ = domain.MixMatcherPatternTypeRegexp
		case Domain_Domain:
			typ = domain.MixMatcherPatternTypeDomain
		case Domain_Full:
			typ = domain.MixMatcherPatternTypeFull
		default:
			return fmt.Errorf("invalid v2ray Domain_Type %d", d.Type)
		}

		err = m.AddElem(typ, d.Value, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func LoadGeoSiteFromDAT(file, countryCode string) (*GeoSite, error) {
	geoSiteList, err := LoadGeoSiteList(file)
	if err != nil {
		return nil, err
	}

	countryCode = strings.ToLower(countryCode)
	entry := geoSiteList.GetEntry()
	for i := range entry {
		if strings.ToLower(entry[i].CountryCode) == countryCode {
			return entry[i], nil
		}
	}

	return nil, fmt.Errorf("can not find category %s in %s", countryCode, file)
}

var geoSiteCache = load_cache.GetCache().NewNamespace()

func LoadGeoSiteList(file string) (*GeoSiteList, error) {
	// load from cache
	v, _ := geoSiteCache.Get(file)
	if geoSiteList, ok := v.(*GeoSiteList); ok {
		return geoSiteList, nil
	}

	// load from disk
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	geoSiteList := new(GeoSiteList)
	if err := proto.Unmarshal(raw, geoSiteList); err != nil {
		return nil, err
	}

	// cache the file
	geoSiteCache.Store(file, geoSiteList)
	time.AfterFunc(time.Second*15, func() { // remove it after 15s
		geoSiteCache.Remove(file)
	})
	return geoSiteList, nil
}
