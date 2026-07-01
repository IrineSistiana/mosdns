package geoip

import (
	"net/netip"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestDecodeGeoIPList(t *testing.T) {
	b := appendMessage(nil, 1, geoIP("CN", false,
		cidr(netip.MustParseAddr("1.0.0.0"), 24),
		cidr(netip.MustParseAddr("2001:db8::"), 32),
	))
	b = appendMessage(b, 1, geoIP("TEST", true,
		cidr(netip.MustParseAddr("10.0.0.0"), 8),
	))

	matchers, err := DecodeGeoIPList(b)
	if err != nil {
		t.Fatal(err)
	}

	cn := matchers["cn"]
	if cn == nil {
		t.Fatal("missing cn matcher")
	}
	if !cn.Match(netip.MustParseAddr("1.0.0.1")) {
		t.Fatal("cn matcher did not match IPv4 prefix")
	}
	if !cn.Match(netip.MustParseAddr("2001:db8::1")) {
		t.Fatal("cn matcher did not match IPv6 prefix")
	}
	if cn.Match(netip.MustParseAddr("2.0.0.1")) {
		t.Fatal("cn matcher unexpectedly matched different IPv4 prefix")
	}

	test := matchers["test"]
	if test == nil {
		t.Fatal("missing test matcher")
	}
	if test.Match(netip.MustParseAddr("10.0.0.1")) {
		t.Fatal("inverse matcher unexpectedly matched excluded prefix")
	}
	if !test.Match(netip.MustParseAddr("11.0.0.1")) {
		t.Fatal("inverse matcher did not match non-excluded prefix")
	}
}

func geoIP(code string, inverse bool, cidrs ...[]byte) []byte {
	var b []byte
	b = appendString(b, 1, code)
	for _, cidr := range cidrs {
		b = appendMessage(b, 2, cidr)
	}
	if inverse {
		b = appendVarint(b, 3, 1)
	}
	return b
}

func cidr(addr netip.Addr, bits uint64) []byte {
	var b []byte
	if addr.Is4() {
		a := addr.As4()
		b = appendBytes(b, 1, a[:])
	} else {
		a := addr.As16()
		b = appendBytes(b, 1, a[:])
	}
	return appendVarint(b, 2, bits)
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
