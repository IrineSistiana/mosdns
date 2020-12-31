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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"v2ray.com/core/app/router"
)

var matcherCache = utils.NewCache()

const (
	cacheTTL = time.Second * 30
)

// ParseValueFunc parses additional `attr` to an interface. The given []string could have a 0 length or is nil.
type ParseValueFunc func([]string) (interface{}, error)

// FilterRecordFunc determines whether a record is acceptable. The given []string could have a 0 length or is nil.
type FilterRecordFunc func([]string) (accept bool, err error)

// LoadFormFile loads data from file.
func (m *MixMatcher) LoadFormFile(file string, filterRecord FilterRecordFunc, parseValue ParseValueFunc) error {
	var err error
	if tmp := strings.SplitN(file, ":", 2); len(tmp) == 2 { // is a v2ray data file
		filePath := tmp[0]
		countryCode := tmp[1]
		err = m.LoadFormDAT(filePath, countryCode, filterRecord, parseValue)
	} else { // is a text file
		err = m.LoadFormTextFile(file, filterRecord, parseValue)
	}
	if err != nil {
		return err
	}

	return nil
}

// LoadFormFileAsV2Matcher loads data from file.
// File can be a text file or a v2ray data file.
// v2ray data file needs to specify the data category by using ':', e.g. 'geosite:cn'
// v2ray data file can also have multiple @attr. e.g. 'geosite:cn@attr1@attr2'.
// Only the domain with all of the @attr will be used.
func (m *MixMatcher) LoadFormFileAsV2Matcher(file string) error {
	var err error
	if tmp := strings.SplitN(file, ":", 2); len(tmp) == 2 { // is a v2ray data file
		filePath := tmp[0]
		tmp := strings.Split(tmp[1], "@")
		countryCode := tmp[0]
		wantedAttr := tmp[1:]
		filterFunc := func(attr []string) (accept bool, err error) {
			return mustHaveAttr(attr, wantedAttr), nil
		}
		err = m.LoadFormDAT(filePath, countryCode, filterFunc, nil)
	} else { // is a text file
		err = m.LoadFormTextFile(file, nil, nil)
	}
	if err != nil {
		return err
	}

	return nil
}

// BatchLoadMixMatcher loads multiple files using MixMatcher.LoadFormFile
func BatchLoadMixMatcher(f []string, filterRecord FilterRecordFunc, parseValue ParseValueFunc) (Matcher, error) {
	if len(f) == 0 {
		return nil, errors.New("no file to load")
	}

	m := NewMixMatcher()
	for _, file := range f {
		err := m.LoadFormFile(file, filterRecord, parseValue)
		if err != nil {
			return nil, fmt.Errorf("failed to load file %s: %w", file, err)
		}
	}
	return m, nil
}

// BatchLoadMixMatcherV2Matcher loads multiple files using MixMatcher.LoadFormFileAsV2Matcher
func BatchLoadMixMatcherV2Matcher(f []string) (Matcher, error) {
	if len(f) == 0 {
		return nil, errors.New("no file to load")
	}

	m := NewMixMatcher()
	for _, file := range f {
		err := m.LoadFormFileAsV2Matcher(file)
		if err != nil {
			return nil, fmt.Errorf("failed to load file %s: %w", file, err)
		}
	}
	return m, nil
}

func (m *MixMatcher) LoadFormTextFile(file string, filterRecord FilterRecordFunc, parseValue ParseValueFunc) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	return m.LoadFormTextReader(bytes.NewBuffer(data), filterRecord, parseValue)
}

func (m *MixMatcher) LoadFormTextReader(r io.Reader, filterRecord FilterRecordFunc, parseValue ParseValueFunc) error {
	lineCounter := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineCounter++
		err := m.LoadFormText(scanner.Text(), filterRecord, parseValue)
		if err != nil {
			return fmt.Errorf("line %d: %v", lineCounter, err)
		}
	}
	return nil
}

var typeStrToDomainType = map[string]router.Domain_Type{
	"":        router.Domain_Domain,
	"domain":  router.Domain_Domain,
	"keyword": router.Domain_Plain,
	"regexp":  router.Domain_Regex,
	"full":    router.Domain_Full,
}

func (m *MixMatcher) LoadFormText(s string, filterRecord FilterRecordFunc, parseValue ParseValueFunc) error {
	t := utils.RemoveComment(s, "#")
	e := utils.SplitLine(t)

	if len(e) == 0 {
		return nil
	}

	pattern := e[0]
	kv := strings.SplitN(pattern, ":", 2)
	var typStr string
	var str string
	if len(kv) == 1 {
		str = kv[0]
	} else {
		typStr = kv[0]
		str = kv[1]
	}

	typ, ok := typeStrToDomainType[typStr]
	if ok {
		var v interface{}
		var err error
		if filterRecord != nil {
			accept, err := filterRecord(e[1:])
			if err != nil {
				return err
			}
			if !accept {
				return nil
			}
		}

		if parseValue != nil {
			v, err = parseValue(e[1:])
			if err != nil {
				return err
			}
		}
		return m.AddElem(typ, str, v)
	} else {
		return fmt.Errorf("unexpected pattern type %s", typStr)
	}
}

func (m *MixMatcher) LoadFormDAT(file, countryCode string, filterRecord FilterRecordFunc, parseValue ParseValueFunc) error {
	domains, err := loadV2DomainsFromDAT(file, countryCode)
	if err != nil {
		return err
	}

	for _, d := range domains {
		attr := make([]string, 0, len(d.Attribute))
		for _, a := range d.Attribute {
			attr = append(attr, a.Key)
		}

		if filterRecord != nil {
			accept, err := filterRecord(attr)
			if err != nil {
				return err
			}
			if !accept {
				return nil
			}
		}

		var v interface{}
		var err error
		if parseValue != nil {
			v, err = parseValue(attr)
			if err != nil {
				return err
			}
		}

		err = m.AddElem(d.Type, d.Value, v)
		if err != nil {
			return err
		}
	}
	return nil
}

// mustHaveAttr checks if attr has all wanted attrs.
func mustHaveAttr(attr, wanted []string) bool {
	if len(wanted) == 0 {
		return true
	}
	if len(attr) == 0 {
		return false
	}

	for _, w := range wanted {
		ok := false
		for _, got := range attr {
			if got == w {
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

func NewV2MatcherFromFile(file, countryCode string) (Matcher, error) {
	domains, err := loadV2DomainsFromDAT(file, countryCode)
	if err != nil {
		return nil, err
	}
	return NewV2Matcher(domains)
}

func loadV2DomainsFromDAT(file, countryCode string) ([]*router.Domain, error) {
	geoSite, err := loadGeoSiteFromDAT(file, countryCode)
	if err != nil {
		return nil, err
	}
	return geoSite.GetDomain(), nil
}

func loadGeoSiteFromDAT(file, countryCode string) (*router.GeoSite, error) {
	geoSiteList, err := loadGeoSiteList(file)
	if err != nil {
		return nil, err
	}

	countryCode = strings.ToUpper(countryCode)
	entry := geoSiteList.GetEntry()
	for i := range entry {
		if strings.ToUpper(entry[i].CountryCode) == countryCode {
			return entry[i], nil
		}
	}

	return nil, fmt.Errorf("can not find category %s in %s", countryCode, file)
}

func loadGeoSiteList(file string) (*router.GeoSiteList, error) {
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
