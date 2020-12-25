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
	"github.com/miekg/dns"
	"io"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/golang/protobuf/proto"
	"v2ray.com/core/app/router"
)

var matcherCache = utils.NewCache()

const (
	cacheTTL = time.Second * 30
)

// NewMixMatcherFormFile loads a list matcher or a v2fly matcher from file.
// if file has a ':' and has format like 'geosite:cn', a v2fly matcher will be returned.
func NewMixMatcherFormFile(file string) (Matcher, error) {
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
		m, err = NewMixMatcherFormTextDAT(tmp[0], tmp[1]) // file and tag
	} else {
		m, err = NewMixMatcherFormTextFile(file, true)
	}
	if err != nil {
		return nil, err
	}

	matcherCache.Put(file, m, cacheTTL)
	return m, nil
}

func NewMixMatcherFormTextFile(file string, continueOnInvalidString bool) (Matcher, error) {
	data, raw, err := matcherCache.LoadFromCacheOrRawDisk(file)
	if err != nil {
		return nil, err
	}

	if m, ok := data.(Matcher); ok {
		return m, nil
	}
	m, err := NewMixMatcherFormTextReader(bytes.NewBuffer(raw), continueOnInvalidString)
	if err != nil {
		return nil, err
	}
	matcherCache.Put(file, m, cacheTTL)
	return m, nil
}

func NewMixMatcherFormTextReader(r io.Reader, continueOnInvalidString bool) (Matcher, error) {
	mixMatcher := NewMixMatcher()

	lineCounter := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineCounter++
		line := strings.TrimSpace(scanner.Text())

		//ignore lines begin with # and empty lines
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		var err error
		switch {
		case strings.HasPrefix(line, "domain:"):
			s := line[len("domain:"):]
			err = mixMatcher.AddElem(router.Domain_Domain, dns.Fqdn(s), nil)
		case strings.HasPrefix(line, "keyword:"):
			s := line[len("keyword:"):]
			err = mixMatcher.AddElem(router.Domain_Plain, s, nil)
		case strings.HasPrefix(line, "regexp:"):
			s := line[len("regexp:"):]
			err = mixMatcher.AddElem(router.Domain_Regex, s, nil)
		case strings.HasPrefix(line, "full:"):
			s := line[len("full:"):]
			err = mixMatcher.AddElem(router.Domain_Full, dns.Fqdn(s), nil)
		default:
			err = mixMatcher.AddElem(router.Domain_Domain, dns.Fqdn(line), nil)
		}
		if err != nil {
			if continueOnInvalidString {
				mlog.Entry().Warnf("invalid record [%s] at line %d", line, lineCounter)
			} else {
				return nil, fmt.Errorf("invalid record [%s] at line %d", line, lineCounter)
			}
		}
	}

	return mixMatcher, nil
}

func NewMixMatcherFormTextDAT(file, tag string) (Matcher, error) {
	domains, err := loadV2DomainsFromDAT(file, tag)
	if err != nil {
		return nil, err
	}

	mixMatcher := NewMixMatcher()
	for _, d := range domains {
		err := mixMatcher.AddElem(d.Type, d.Value, nil)
		if err != nil {
			return nil, err
		}
	}
	return mixMatcher, nil
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
