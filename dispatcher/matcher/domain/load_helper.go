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

package domain

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"io"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/dispatcher/logger"
	"github.com/golang/protobuf/proto"
	"github.com/miekg/dns"
	"v2ray.com/core/app/router"
)

var matcherCache = utils.NewCache()

const (
	cacheTTL = time.Second * 30
)

// NewDomainMatcherFormFile loads a list matcher or a v2fly matcher from file.
// if file has a ':' and has format like 'geosite:cn', a v2fly matcher will be returned.
func NewDomainMatcherFormFile(file string) (Matcher, error) {
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
		m, err = NewV2MatcherFromFile(tmp[0], tmp[1]) // file and tag
	} else {
		m, err = NewDomainListMatcherFormFile(file, true)
	}
	if err != nil {
		return nil, err
	}

	matcherCache.Put(file, m, cacheTTL)
	return m, nil
}

func NewDomainListMatcherFormFile(file string, continueOnInvalidString bool) (Matcher, error) {
	data, raw, err := matcherCache.LoadFromCacheOrRawDisk(file)
	if err != nil {
		return nil, err
	}

	if m, ok := data.(Matcher); ok {
		return m, nil
	}
	m, err := NewDomainListMatcherFormReader(bytes.NewBuffer(raw), continueOnInvalidString)
	if err != nil {
		return nil, err
	}
	matcherCache.Put(file, m, cacheTTL)
	return m, nil
}

func NewDomainListMatcherFormReader(r io.Reader, continueOnInvalidString bool) (Matcher, error) {
	l := NewListMatcher()

	lineCounter := 0
	s := bufio.NewScanner(r)
	for s.Scan() {
		lineCounter++
		line := strings.TrimSpace(s.Text())

		//ignore lines begin with # and empty lines
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		fqdn := dns.Fqdn(line)
		if _, ok := dns.IsDomainName(fqdn); !ok {
			if continueOnInvalidString {
				logger.GetStd().Warnf("NewMatcherFormReader: invalid domain [%s] at line %d", line, lineCounter)
			} else {
				return nil, fmt.Errorf("invalid domain [%s] at line %d", line, lineCounter)
			}
		}
		l.Add(fqdn)

	}
	return l, nil
}

func NewV2MatcherFromFile(file, tag string) (Matcher, error) {
	domains, err := loadV2DomainsFromDAT(file, tag)
	if err != nil {
		return nil, err
	}
	return NewV2Matcher(domains)
}

func loadV2DomainsFromDAT(file, tag string) ([]*router.Domain, error) {
	geoSite, err := loadGeoSiteFromDAT(file, tag)
	if err != nil {
		return nil, err
	}
	return geoSite.GetDomain(), nil
}

func loadGeoSiteFromDAT(file, tag string) (*router.GeoSite, error) {
	geoSiteList, err := loadGeoSiteListFromDAT(file)
	if err != nil {
		return nil, err
	}

	entry := geoSiteList.GetEntry()
	for i := range entry {
		if strings.ToUpper(entry[i].CountryCode) == strings.ToUpper(tag) {
			return entry[i], nil
		}
	}

	return nil, fmt.Errorf("can not find tag %s in %s", tag, file)
}

func loadGeoSiteListFromDAT(file string) (*router.GeoSiteList, error) {
	data, raw, err := matcherCache.LoadFromCacheOrRawDisk(file)
	if err != nil {
		return nil, err
	}
	// load from cache
	if geoSiteList, ok := data.(*router.GeoSiteList); ok {
		return geoSiteList, nil
	}

	// load from disk
	geoSiteList := new(router.GeoSiteList)
	if err := proto.Unmarshal(raw, geoSiteList); err != nil {
		return nil, err
	}

	// cache the file
	matcherCache.Put(file, geoSiteList, cacheTTL)
	return geoSiteList, nil
}
