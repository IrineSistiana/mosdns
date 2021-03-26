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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/load_cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
)

var matcherCache = load_cache.GetCache().NewNamespace()

// ProcessAttrFunc processes the additional attributions. The given []string could have a 0 length or is nil.
type ProcessAttrFunc func([]string) (v interface{}, accept bool, err error)

// Load loads data from a entry.
// If entry begin with "ext:", Load loads the file by using LoadFromFile.
// Else it loads the entry as a text pattern by using LoadFromText.
func Load(m Matcher, entry string, processAttr ProcessAttrFunc) error {
	s1, s2, ok := utils.SplitString2(entry, ":")
	if ok && s1 == "ext" {
		return LoadFromFile(m, s2, processAttr)
	}
	return LoadFromText(m, entry, processAttr)
}

// BatchLoadMatcher loads multiple files using Load.
func BatchLoadMatcher(m Matcher, f []string, processAttr ProcessAttrFunc) error {
	for _, file := range f {
		err := Load(m, file, processAttr)
		if err != nil {
			return fmt.Errorf("failed to load file %s: %w", file, err)
		}
	}
	return nil
}

// LoadFromFile loads data from a file.
// v2ray data file can also have multiple @attr. e.g. 'geosite.dat:cn@attr1@attr2'.
// Only the record with all of the @attr will be loaded.
func LoadFromFile(m Matcher, file string, processAttr ProcessAttrFunc) error {
	var err error
	if tmp := strings.SplitN(file, ":", 2); len(tmp) == 2 { // is a v2ray data file
		mixMatcher, ok := m.(*MixMatcher)
		if !ok {
			return errors.New("only MixMatcher can load v2ray data file")
		}
		filePath := tmp[0]
		tmp := strings.Split(tmp[1], "@")
		countryCode := tmp[0]
		wantedAttr := tmp[1:]
		v2ProcessAttr := func(attr []string) (v interface{}, accept bool, err error) {
			v2Accept := mustHaveAttr(attr, wantedAttr)
			if v2Accept {
				if processAttr != nil {
					return processAttr(attr)
				}
				return nil, true, nil
			}
			return nil, false, nil
		}
		err = mixMatcher.LoadFromDAT(filePath, countryCode, v2ProcessAttr)
	} else { // is a text file
		err = LoadFromTextFile(m, file, processAttr)
	}
	if err != nil {
		return err
	}

	return nil
}

func LoadFromTextFile(m Matcher, file string, processAttr ProcessAttrFunc) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	return LoadFromTextReader(m, bytes.NewBuffer(data), processAttr)
}

func LoadFromTextReader(m Matcher, r io.Reader, processAttr ProcessAttrFunc) error {
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

func LoadFromText(m Matcher, s string, processAttr ProcessAttrFunc) error {
	if processAttr != nil {
		e := utils.SplitLine(s)
		pattern := e[0]
		attr := e[1:]
		v, accept, err := processAttr(attr)
		if err != nil {
			return err
		}
		if !accept {
			return nil
		}
		return m.Add(pattern, v)
	}

	pattern := utils.RemoveComment(s, " ")
	return m.Add(pattern, nil)
}

func (m *MixMatcher) LoadFromDAT(file, countryCode string, processAttr ProcessAttrFunc) error {
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

		var typ MixMatcherPatternType
		switch d.Type {
		case v2data.Domain_Plain:
			typ = MixMatcherPatternTypeKeyword
		case v2data.Domain_Regex:
			typ = MixMatcherPatternTypeRegexp
		case v2data.Domain_Domain:
			typ = MixMatcherPatternTypeDomain
		case v2data.Domain_Full:
			typ = MixMatcherPatternTypeFull
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

func LoadGeoSiteFromDAT(file, countryCode string) (*v2data.GeoSite, error) {
	geoSiteList, err := LoadGeoSiteList(file)
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

func LoadGeoSiteList(file string) (*v2data.GeoSiteList, error) {
	// load from cache
	v, _ := matcherCache.Get(file)
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
	matcherCache.Store(file, geoSiteList)
	time.AfterFunc(time.Second*15, func() { // remove it after 15s
		matcherCache.Remove(file)
	})
	return geoSiteList, nil
}
