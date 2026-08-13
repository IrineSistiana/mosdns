/*
 * Copyright (C) 2026
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

package domain_set

import (
	"fmt"
	"os"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"google.golang.org/protobuf/encoding/protowire"
)

// The wire format is defined by v2fly-core/app/router/routercommon/common.proto.
// Only the fields used by domain-list-community's dlc.dat are decoded here. This
// avoids making the full v2fly-core runtime a dependency of mosdns.
const (
	geoSiteListGeoSiteField = 1
	geoSiteCountryCodeField = 1
	geoSiteDomainField      = 2
	domainTypeField         = 1
	domainValueField        = 2
)

const (
	geoSiteDomainPlain      = 0
	geoSiteDomainRegex      = 1
	geoSiteDomainRootDomain = 2
	geoSiteDomainFull       = 3
)

// LoadGeoSiteSources loads the selected GeoSite tags from every source.
func LoadGeoSiteSources(sources []GeoSiteSource, m *domain.MixMatcher[struct{}]) error {
	for i, source := range sources {
		if strings.TrimSpace(source.File) == "" {
			return fmt.Errorf("geosite source #%d has an empty file path", i)
		}
		if len(source.Tags) == 0 {
			return fmt.Errorf("geosite source #%d %s has no tags", i, source.File)
		}
		if err := LoadGeoSiteFile(source.File, source.Tags, m); err != nil {
			return fmt.Errorf("failed to load geosite source #%d %s, %w", i, source.File, err)
		}
	}
	return nil
}

// LoadGeoSiteFile loads the requested GeoSite tags from a GeoSite protobuf file.
func LoadGeoSiteFile(file string, tags []string, m *domain.MixMatcher[struct{}]) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return LoadGeoSiteData(b, tags, m)
}

// LoadGeoSiteData loads the requested GeoSite tags from GeoSite protobuf data.
func LoadGeoSiteData(b []byte, tags []string, m *domain.MixMatcher[struct{}]) error {
	wanted := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			return fmt.Errorf("empty geosite tag")
		}
		wanted[tag] = struct{}{}
	}

	found := make(map[string]struct{}, len(wanted))
	for len(b) > 0 {
		number, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return fmt.Errorf("invalid geosite data: %v", protowire.ParseError(n))
		}
		b = b[n:]
		if number == geoSiteListGeoSiteField && typ == protowire.BytesType {
			geoSite, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return fmt.Errorf("invalid geosite entry: %v", protowire.ParseError(n))
			}
			b = b[n:]
			tag, domains, err := parseGeoSite(geoSite)
			if err != nil {
				return err
			}
			if _, ok := wanted[tag]; ok {
				found[tag] = struct{}{}
				for _, d := range domains {
					if err := addGeoSiteDomain(m, d); err != nil {
						return fmt.Errorf("tag %q: %w", tag, err)
					}
				}
			}
			continue
		}

		n = protowire.ConsumeFieldValue(number, typ, b)
		if n < 0 {
			return fmt.Errorf("invalid geosite data: %v", protowire.ParseError(n))
		}
		b = b[n:]
	}

	missing := make([]string, 0)
	for tag := range wanted {
		if _, ok := found[tag]; !ok {
			missing = append(missing, tag)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("geosite tags not found: %s", strings.Join(missing, ", "))
	}
	return nil
}

type geoSiteDomain struct {
	typ   uint64
	value string
}

func parseGeoSite(b []byte) (string, []geoSiteDomain, error) {
	var tag string
	var domains []geoSiteDomain
	for len(b) > 0 {
		number, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", nil, fmt.Errorf("invalid geosite entry: %v", protowire.ParseError(n))
		}
		b = b[n:]
		switch {
		case number == geoSiteCountryCodeField && typ == protowire.BytesType:
			value, n := protowire.ConsumeString(b)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid geosite tag: %v", protowire.ParseError(n))
			}
			tag = strings.ToLower(value)
			b = b[n:]
		case number == geoSiteDomainField && typ == protowire.BytesType:
			value, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid geosite domain: %v", protowire.ParseError(n))
			}
			b = b[n:]
			domain, err := parseGeoSiteDomain(value)
			if err != nil {
				return "", nil, err
			}
			domains = append(domains, domain)
		default:
			n = protowire.ConsumeFieldValue(number, typ, b)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid geosite entry: %v", protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	if tag == "" {
		return "", nil, fmt.Errorf("geosite entry has an empty tag")
	}
	return tag, domains, nil
}

func parseGeoSiteDomain(b []byte) (geoSiteDomain, error) {
	d := geoSiteDomain{}
	for len(b) > 0 {
		number, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return d, fmt.Errorf("invalid geosite domain: %v", protowire.ParseError(n))
		}
		b = b[n:]
		switch {
		case number == domainTypeField && typ == protowire.VarintType:
			value, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return d, fmt.Errorf("invalid geosite domain type: %v", protowire.ParseError(n))
			}
			d.typ = value
			b = b[n:]
		case number == domainValueField && typ == protowire.BytesType:
			value, n := protowire.ConsumeString(b)
			if n < 0 {
				return d, fmt.Errorf("invalid geosite domain value: %v", protowire.ParseError(n))
			}
			d.value = value
			b = b[n:]
		default:
			n = protowire.ConsumeFieldValue(number, typ, b)
			if n < 0 {
				return d, fmt.Errorf("invalid geosite domain: %v", protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	if d.value == "" {
		return d, fmt.Errorf("geosite domain has an empty value")
	}
	return d, nil
}

func addGeoSiteDomain(m *domain.MixMatcher[struct{}], d geoSiteDomain) error {
	var pattern string
	switch d.typ {
	case geoSiteDomainPlain:
		pattern = domain.MatcherKeyword + ":" + d.value
	case geoSiteDomainRegex:
		pattern = domain.MatcherRegexp + ":" + d.value
	case geoSiteDomainRootDomain:
		pattern = domain.MatcherDomain + ":" + d.value
	case geoSiteDomainFull:
		pattern = domain.MatcherFull + ":" + d.value
	default:
		return fmt.Errorf("unsupported geosite domain type %d", d.typ)
	}
	if err := m.Add(pattern, struct{}{}); err != nil {
		return fmt.Errorf("invalid %s rule %q: %w", strings.SplitN(pattern, ":", 2)[0], d.value, err)
	}
	return nil
}
