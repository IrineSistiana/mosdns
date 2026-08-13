package domain_set

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestLoadGeoSiteData(t *testing.T) {
	data := makeGeoSiteList(
		geoSiteFixture{
			tag: "CN",
			domains: []geoSiteDomain{
				{typ: geoSiteDomainRootDomain, value: "example.cn"},
				{typ: geoSiteDomainFull, value: "only.example.cn"},
				{typ: geoSiteDomainPlain, value: "keyword"},
				{typ: geoSiteDomainRegex, value: "^regex\\.example\\.cn$"},
			},
		},
		geoSiteFixture{tag: "us", domains: []geoSiteDomain{{typ: geoSiteDomainRootDomain, value: "example.us"}}},
	)

	m := domain.NewDomainMixMatcher()
	if err := LoadGeoSiteData(data, []string{"cn"}, m); err != nil {
		t.Fatalf("LoadGeoSiteData() error = %v", err)
	}

	for _, name := range []string{"example.cn", "sub.example.cn", "only.example.cn", "keyword-domain.test", "regex.example.cn"} {
		if _, ok := m.Match(name); !ok {
			t.Errorf("Match(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"otherexample.cn", "sub.example.us"} {
		if _, ok := m.Match(name); ok {
			t.Errorf("Match(%q) = true, want false", name)
		}
	}
}

func TestLoadGeoSiteDataErrors(t *testing.T) {
	t.Run("missing tag", func(t *testing.T) {
		m := domain.NewDomainMixMatcher()
		err := LoadGeoSiteData(makeGeoSiteList(geoSiteFixture{tag: "cn"}), []string{"us"}, m)
		if err == nil || err.Error() != "geosite tags not found: us" {
			t.Fatalf("LoadGeoSiteData() error = %v, want missing-tag error", err)
		}
	})

	t.Run("invalid data", func(t *testing.T) {
		m := domain.NewDomainMixMatcher()
		if err := LoadGeoSiteData([]byte{0x0a, 0x80}, []string{"cn"}, m); err == nil {
			t.Fatal("LoadGeoSiteData() error = nil, want error")
		}
	})

	t.Run("unsupported domain type", func(t *testing.T) {
		m := domain.NewDomainMixMatcher()
		err := LoadGeoSiteData(makeGeoSiteList(geoSiteFixture{tag: "cn", domains: []geoSiteDomain{{typ: 99, value: "example.cn"}}}), []string{"cn"}, m)
		if err == nil {
			t.Fatal("LoadGeoSiteData() error = nil, want error")
		}
	})
}

func TestLoadGeoSiteSources(t *testing.T) {
	file := filepath.Join(t.TempDir(), "dlc.dat")
	if err := os.WriteFile(file, makeGeoSiteList(geoSiteFixture{tag: "cn", domains: []geoSiteDomain{{typ: geoSiteDomainRootDomain, value: "example.cn"}}}), 0o600); err != nil {
		t.Fatal(err)
	}

	m := domain.NewDomainMixMatcher()
	if err := LoadGeoSiteSources([]GeoSiteSource{{File: file, Tags: []string{"cn"}}}, m); err != nil {
		t.Fatalf("LoadGeoSiteSources() error = %v", err)
	}
	if _, ok := m.Match("sub.example.cn"); !ok {
		t.Fatal("loaded source did not match selected domain")
	}
}

type geoSiteFixture struct {
	tag     string
	domains []geoSiteDomain
}

func makeGeoSiteList(sites ...geoSiteFixture) []byte {
	var list []byte
	for _, site := range sites {
		var entry []byte
		entry = protowire.AppendTag(entry, geoSiteCountryCodeField, protowire.BytesType)
		entry = protowire.AppendString(entry, site.tag)
		for _, d := range site.domains {
			var value []byte
			if d.typ != geoSiteDomainPlain {
				value = protowire.AppendTag(value, domainTypeField, protowire.VarintType)
				value = protowire.AppendVarint(value, d.typ)
			}
			value = protowire.AppendTag(value, domainValueField, protowire.BytesType)
			value = protowire.AppendString(value, d.value)
			entry = protowire.AppendTag(entry, geoSiteDomainField, protowire.BytesType)
			entry = protowire.AppendBytes(entry, value)
		}
		list = protowire.AppendTag(list, geoSiteListGeoSiteField, protowire.BytesType)
		list = protowire.AppendBytes(list, entry)
	}
	return list
}
