/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package geosite

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"google.golang.org/protobuf/encoding/protowire"
)

const PluginType = "geosite"

const (
	geositeDomainPlain  = 0
	geositeDomainRegex  = 1
	geositeDomainDomain = 2
	geositeDomainFull   = 3
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

func Init(bp *coremain.BP, args any) (any, error) {
	return NewGeosite(bp, args.(*Args))
}

type Args struct {
	File string `yaml:"file"`
}

var _ data_provider.GeositeMatcherProvider = (*Geosite)(nil)

type Geosite struct {
	mu       sync.Mutex
	sites    map[string][]geositeDomain
	matchers map[string]domain.Matcher[struct{}]
}

func (g *Geosite) GetGeositeMatcher(code string) (domain.Matcher[struct{}], bool) {
	name, attrs := parseCode(code)
	if len(name) == 0 {
		return nil, false
	}

	cacheKey := name
	if len(attrs) > 0 {
		cacheKey += "@" + strings.Join(attrs, "@")
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if m, ok := g.matchers[cacheKey]; ok {
		return m, true
	}

	domains, ok := g.sites[name]
	if !ok {
		return nil, false
	}
	m, err := buildMatcher(domains, attrs)
	if err != nil {
		return nil, false
	}
	g.matchers[cacheKey] = m
	return m, true
}

func NewGeosite(bp *coremain.BP, args *Args) (*Geosite, error) {
	sites, err := LoadGeosite(args.File)
	if err != nil {
		return nil, err
	}
	return &Geosite{
		sites:    sites,
		matchers: make(map[string]domain.Matcher[struct{}]),
	}, nil
}

type geositeDomain struct {
	typ   uint64
	value string
	attrs map[string]struct{}
}

func LoadGeosite(file string) (map[string][]geositeDomain, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return DecodeGeositeList(b)
}

func DecodeGeositeList(b []byte) (map[string][]geositeDomain, error) {
	sites := make(map[string][]geositeDomain)
	if err := forEachField(b, func(num protowire.Number, typ protowire.Type, value []byte) error {
		if num != 1 || typ != protowire.BytesType {
			return nil
		}
		code, domains, err := decodeGeosite(value)
		if err != nil {
			return err
		}
		if len(code) != 0 {
			sites[strings.ToLower(code)] = domains
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return sites, nil
}

func decodeGeosite(b []byte) (string, []geositeDomain, error) {
	var code string
	var domains []geositeDomain
	if err := forEachField(b, func(num protowire.Number, typ protowire.Type, value []byte) error {
		switch num {
		case 1:
			if typ == protowire.BytesType {
				code = string(value)
			}
		case 2:
			if typ != protowire.BytesType {
				return nil
			}
			d, err := decodeGeositeDomain(value)
			if err != nil {
				return err
			}
			if len(d.value) != 0 {
				domains = append(domains, d)
			}
		}
		return nil
	}); err != nil {
		return "", nil, err
	}
	return code, domains, nil
}

func decodeGeositeDomain(b []byte) (geositeDomain, error) {
	d := geositeDomain{typ: geositeDomainPlain}
	if err := forEachField(b, func(num protowire.Number, typ protowire.Type, value []byte) error {
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return nil
			}
			v, n := protowire.ConsumeVarint(value)
			if n < 0 {
				return protowire.ParseError(n)
			}
			d.typ = v
		case 2:
			if typ == protowire.BytesType {
				d.value = string(value)
			}
		case 3:
			if typ != protowire.BytesType {
				return nil
			}
			key, err := decodeGeositeAttribute(value)
			if err != nil {
				return err
			}
			if len(key) != 0 {
				if d.attrs == nil {
					d.attrs = make(map[string]struct{})
				}
				d.attrs[strings.ToLower(key)] = struct{}{}
			}
		}
		return nil
	}); err != nil {
		return geositeDomain{}, err
	}
	return d, nil
}

func decodeGeositeAttribute(b []byte) (string, error) {
	var key string
	if err := forEachField(b, func(num protowire.Number, typ protowire.Type, value []byte) error {
		if num == 1 && typ == protowire.BytesType {
			key = string(value)
		}
		return nil
	}); err != nil {
		return "", err
	}
	return key, nil
}

func parseCode(code string) (string, []string) {
	parts := strings.Split(strings.ToLower(code), "@")
	name := strings.TrimSpace(parts[0])
	var attrs []string
	for _, attr := range parts[1:] {
		attr = strings.TrimSpace(attr)
		if len(attr) != 0 {
			attrs = append(attrs, attr)
		}
	}
	return name, attrs
}

func buildMatcher(domains []geositeDomain, attrs []string) (domain.Matcher[struct{}], error) {
	m := domain.NewDomainMixMatcher()
	for _, d := range domains {
		if !matchAttrs(d, attrs) {
			continue
		}
		if err := addGeositeDomain(m, d); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func matchAttrs(d geositeDomain, attrs []string) bool {
	for _, attr := range attrs {
		if _, ok := d.attrs[attr]; !ok {
			return false
		}
	}
	return true
}

func addGeositeDomain(m *domain.MixMatcher[struct{}], d geositeDomain) error {
	var matcher string
	switch d.typ {
	case geositeDomainPlain:
		matcher = domain.MatcherKeyword
	case geositeDomainRegex:
		matcher = domain.MatcherRegexp
	case geositeDomainDomain:
		matcher = domain.MatcherDomain
	case geositeDomainFull:
		matcher = domain.MatcherFull
	default:
		return fmt.Errorf("unsupported geosite domain type %d", d.typ)
	}
	return m.GetSubMatcher(matcher).Add(d.value, struct{}{})
}

func forEachField(b []byte, f func(num protowire.Number, typ protowire.Type, value []byte) error) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]

		vn := protowire.ConsumeFieldValue(num, typ, b)
		if vn < 0 {
			return protowire.ParseError(vn)
		}
		value := b[:vn]
		if typ == protowire.BytesType {
			var n int
			value, n = protowire.ConsumeBytes(value)
			if n < 0 {
				return protowire.ParseError(n)
			}
		}
		if err := f(num, typ, value); err != nil {
			return err
		}
		b = b[vn:]
	}
	return nil
}
