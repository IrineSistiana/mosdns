package nftset_utils

import (
	"net"
	"testing"
)

func Test_broadcastAddr(t *testing.T) {
	tests := []struct {
		name string
		cidr string
		want string
	}{
		{"1", "192.168.1.1/24", "192.168.1.255"},
		{"2", "192.168.1.1/32", "192.168.1.1"},
		{"3", "192.168.1.1/16", "192.168.255.255"},
		{"4", "192.168.1.1/25", "192.168.1.127"},
		{"5", "2001:db8::68/128", "2001:db8::68"},
		{"6", "2001:db8::68/64", "2001:0DB8:0000:0000:FFFF:FFFF:FFFF:FFFF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ipNet, err := net.ParseCIDR(tt.cidr)
			if err != nil {
				t.Fatal(err)
			}
			want := net.ParseIP(tt.want)
			if got := broadcastAddr(ipNet); !got.Equal(want) {
				t.Errorf("broadcastAddr() = %v, want %v", got, tt.want)
			}
		})
	}
}
