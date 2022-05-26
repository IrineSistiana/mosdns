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

package domain

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/load_cache"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"google.golang.org/protobuf/proto"
	"io"
	"os"
	"reflect"
	"strings"
	"time"
)

// ProcessAttrFunc processes the additional attributions.
type ProcessAttrFunc[T any] func(attr string) (v T, err error)

// Load loads data from an entry.
// If entry begin with "ext:", Load loads the file by using LoadFromFile.
// Else it loads the entry as a text pattern by using LoadFromText.
func Load[T any](m Matcher[T], entry string, processAttr ProcessAttrFunc[T]) error {
	s1, s2, ok := utils.SplitString2(entry, ":")
	if ok && s1 == "ext" {
		return LoadFromFile(m, s2, processAttr)
	}
	return LoadFromText(m, entry, processAttr)
}

// BatchLoad loads multiple files or entries using Load.
func BatchLoad[T any](m Matcher[T], entries []string, processAttr ProcessAttrFunc[T]) error {
	for _, e := range entries {
		err := Load(m, e, processAttr)
		if err != nil {
			return fmt.Errorf("failed to load entry %s: %w", e, err)
		}
	}
	return nil
}

// LoadFromFile loads data from a file.
func LoadFromFile[T any](m Matcher[T], file string, processAttr ProcessAttrFunc[T]) error {
	var err error
	if tmp := strings.SplitN(file, ":", 2); len(tmp) == 2 { // is a v2ray data file
		mixMatcher, ok := m.(*MixMatcher[T])
		if !ok {
			return fmt.Errorf("only MixMatcher can load v2ray data file, got a %s", reflect.ValueOf(m).Type().Name())
		}
		filePath := tmp[0]
		tmp := strings.Split(tmp[1], "@")
		countryCode := tmp[0]
		wantedAttr := tmp[1:]
		err = LoadMixMatcherFromDAT(mixMatcher, filePath, countryCode, wantedAttr)
	} else { // is a text file
		err = LoadFromTextFile(m, file, processAttr)
	}
	if err != nil {
		return err
	}

	return nil
}

func LoadFromTextFile[T any](m Matcher[T], file string, processAttr ProcessAttrFunc[T]) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	return LoadFromTextReader(m, bytes.NewReader(data), processAttr)
}

func LoadFromTextReader[T any](m Matcher[T], r io.Reader, processAttr ProcessAttrFunc[T]) error {
	lineCounter := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineCounter++
		s := scanner.Text()
		s = utils.RemoveComment(s, "#")
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			continue
		}

		err := LoadFromText(m, s, processAttr)
		if err != nil {
			return fmt.Errorf("line %d: %v", lineCounter, err)
		}
	}
	return scanner.Err()
}

func LoadFromText[T any](m Matcher[T], s string, processAttr ProcessAttrFunc[T]) error {
	if processAttr != nil {
		pattern, attr, ok := utils.SplitString2(s, " ")
		if !ok {
			pattern = s
		}
		pattern = strings.TrimSpace(pattern)
		attr = strings.TrimSpace(attr)

		v, err := processAttr(attr)
		if err != nil {
			return err
		}
		return m.Add(pattern, v)
	}

	var zeroT T
	return m.Add(strings.TrimSpace(s), zeroT)
}

func LoadMixMatcherFromDAT[T any](m *MixMatcher[T], file, countryCode string, attrs []string) error {
	geoSite, err := LoadGeoSiteFromDAT(file, countryCode)
	if err != nil {
		return err
	}

	am := make(map[string]struct{})
	for _, attr := range attrs {
		am[attr] = struct{}{}
	}

getDomainLoop:
	for i, d := range geoSite.GetDomain() {
		for _, attr := range d.Attribute {
			if _, ok := am[attr.Key]; !ok {
				continue getDomainLoop
			}
		}

		var subMatcherType string
		switch d.Type {
		case v2data.Domain_Plain:
			subMatcherType = MatcherKeyword
		case v2data.Domain_Regex:
			subMatcherType = MatcherRegexp
		case v2data.Domain_Domain:
			subMatcherType = MatcherDomain
		case v2data.Domain_Full:
			subMatcherType = MatcherFull
		default:
			return fmt.Errorf("invalid v2ray Domain_Type %d", d.Type)
		}

		sm := m.GetSubMatcher(subMatcherType)
		if sm == nil {
			return fmt.Errorf("invalid MixMatcher, missing submatcher %s", subMatcherType)
		}

		var zeroT T
		if err := sm.Add(d.Value, zeroT); err != nil {
			return fmt.Errorf("failed to load value #%d, %w", i, err)
		}
	}
	return nil
}

func LoadGeoSiteFromDAT(file, countryCode string) (*v2data.GeoSite, error) {
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

func LoadGeoSiteList(file string) (*v2data.GeoSiteList, error) {
	// load from cache
	v, _ := geoSiteCache.Get(file)
	if geoSiteList, ok := v.(*v2data.GeoSiteList); ok {
		return geoSiteList, nil
	}

	// load from disk
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	geoSiteList := new(v2data.GeoSiteList)
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
