package geosite

import (
	"testing"

	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestDecodeGeositeList(t *testing.T) {
	b := appendMessage(nil, 1, geoSite("CN",
		geositeDomainMsg(geositeDomainDomain, "example.com"),
		geositeDomainMsg(geositeDomainFull, "full.example"),
		geositeDomainMsg(geositeDomainPlain, "keyword"),
		geositeDomainMsg(geositeDomainRegex, `^regexp\.example$`),
	))

	sites, err := DecodeGeositeList(b)
	if err != nil {
		t.Fatal(err)
	}
	m, err := buildMatcher(sites["cn"], nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"www.example.com.", true},
		{"full.example", true},
		{"www.full.example", false},
		{"has-keyword.example", true},
		{"regexp.example", true},
		{"regexp.example.org", false},
	}
	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			_, got := m.Match(tt.domain)
			if got != tt.want {
				t.Fatalf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetGeositeMatcherWithAttrs(t *testing.T) {
	g := &Geosite{
		sites: map[string][]geositeDomain{
			"cn": {
				{typ: geositeDomainDomain, value: "cn.example", attrs: map[string]struct{}{"cn": {}}},
				{typ: geositeDomainDomain, value: "ads.example", attrs: map[string]struct{}{"ads": {}}},
				{typ: geositeDomainDomain, value: "plain.example"},
			},
		},
		matchers: make(map[string]domain.Matcher[struct{}]),
	}

	m, ok := g.GetGeositeMatcher("cn@cn")
	if !ok {
		t.Fatal("missing cn@cn matcher")
	}
	if _, ok := m.Match("www.cn.example"); !ok {
		t.Fatal("cn@cn matcher did not include cn attr domain")
	}
	if _, ok := m.Match("www.ads.example"); ok {
		t.Fatal("cn@cn matcher included ads attr domain")
	}
	if _, ok := m.Match("www.plain.example"); ok {
		t.Fatal("cn@cn matcher included domain without attr")
	}

	m, ok = g.GetGeositeMatcher("cn")
	if !ok {
		t.Fatal("missing cn matcher")
	}
	if _, ok := m.Match("www.ads.example"); !ok {
		t.Fatal("cn matcher did not include unfiltered ads attr domain")
	}
}

func geoSite(code string, domains ...[]byte) []byte {
	var b []byte
	b = appendString(b, 1, code)
	for _, domain := range domains {
		b = appendMessage(b, 2, domain)
	}
	return b
}

func geositeDomainMsg(typ uint64, value string, attrs ...string) []byte {
	var b []byte
	b = appendVarint(b, 1, typ)
	b = appendString(b, 2, value)
	for _, attr := range attrs {
		b = appendMessage(b, 3, geositeAttr(attr))
	}
	return b
}

func geositeAttr(key string) []byte {
	return appendString(nil, 1, key)
}

func appendString(b []byte, num protowire.Number, s string) []byte {
	return appendBytes(b, num, []byte(s))
}

func appendMessage(b []byte, num protowire.Number, msg []byte) []byte {
	return appendBytes(b, num, msg)
}

func appendBytes(b []byte, num protowire.Number, value []byte) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, value)
}

func appendVarint(b []byte, num protowire.Number, value uint64) []byte {
	b = protowire.AppendTag(b, num, protowire.VarintType)
	return protowire.AppendVarint(b, value)
}
