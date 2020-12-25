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

	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/golang/protobuf/proto"
	"v2ray.com/core/app/router"
)

var matcherCache = utils.NewCache()

const (
	cacheTTL = time.Second * 30
)

// NewMixMatcherFormFile loads a matcher from file.
// File can be a text file or a v2ray data file.
// v2ray data file needs to specify the data category by using ':', e.g. 'geosite:cn'
// v2ray data file can also have multiple @attr. e.g. 'geosite:cn@attr1@attr2'.
// Only the domain with all of the @attr will be used.
func NewMixMatcherFormFile(file string) (Matcher, error) {
	e, ok := matcherCache.Load(file)
	if ok {
		if m, ok := e.(Matcher); ok {
			return m, nil
		}
	}

	var m Matcher
	var err error
	if tmp := strings.SplitN(file, ":", 2); len(tmp) == 2 { // is a v2ray data file
		filePath := tmp[0]
		tmp := strings.Split(tmp[1], "@")
		category := tmp[0]
		attr := tmp[1:]
		m, err = NewMixMatcherFormTextDAT(filePath, category, attr)
	} else { // is a text file
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

		t := strings.Split(line, " ")
		data := t[0]

		// TODO: support @attr

		var err error
		switch {
		case strings.HasPrefix(data, "domain:"):
			s := data[len("domain:"):]
			err = mixMatcher.AddElem(router.Domain_Domain, s, nil)
		case strings.HasPrefix(data, "keyword:"):
			s := data[len("keyword:"):]
			err = mixMatcher.AddElem(router.Domain_Plain, s, nil)
		case strings.HasPrefix(data, "regexp:"):
			s := data[len("regexp:"):]
			err = mixMatcher.AddElem(router.Domain_Regex, s, nil)
		case strings.HasPrefix(data, "full:"):
			s := data[len("full:"):]
			err = mixMatcher.AddElem(router.Domain_Full, s, nil)
		default:
			err = mixMatcher.AddElem(router.Domain_Domain, data, nil)
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

func NewMixMatcherFormTextDAT(file, tag string, attr []string) (Matcher, error) {
	domains, err := loadV2DomainsFromDAT(file, tag)
	if err != nil {
		return nil, err
	}

	mixMatcher := NewMixMatcher()
	for _, d := range domains {
		if len(attr) != 0 && !containAttr(d.Attribute, attr) {
			continue
		}
		err := mixMatcher.AddElem(d.Type, d.Value, nil)
		if err != nil {
			return nil, err
		}
	}
	return mixMatcher, nil
}

// containAttr checks if d has all attrs.
func containAttr(attr []*router.Domain_Attribute, want []string) bool {
	if len(want) == 0 {
		return true
	}
	if len(attr) == 0 {
		return false
	}

	for _, want := range want {
		ok := false
		for _, got := range attr {
			if got.Key == want {
				ok = true
				break
			}
		}
		if !ok { // this attr is not in d.
			return false
		}
	}
	return true
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

	tag = strings.ToUpper(tag)
	entry := geoSiteList.GetEntry()
	for i := range entry {
		if strings.ToUpper(entry[i].CountryCode) == tag {
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
