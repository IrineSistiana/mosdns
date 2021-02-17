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

package netlist

import (
	"bytes"
	"net"
	"reflect"
	"testing"
)

func TestIPNetList_Sort_And_Merge(t *testing.T) {
	raw := `
192.168.0.0/16
192.168.1.1/24 # merged
192.168.9.24/24 # merged
192.168.3.0/24 # merged
192.169.0.0/16
`
	ipNetList := NewList()
	err := LoadFromReader(ipNetList, bytes.NewBufferString(raw))
	if err != nil {
		t.Fatal(err)
	}
	ipNetList.Sort()

	if ipNetList.Len() != 2 {
		t.Fatalf("unexpected length %d", ipNetList.Len())
	}

	tests := []struct {
		name   string
		testIP net.IP
		want   bool
	}{
		{"0", net.IPv4(192, 167, 255, 255), false},
		{"1", net.IPv4(192, 168, 0, 0), true},
		{"2", net.IPv4(192, 168, 1, 1), true},
		{"3", net.IPv4(192, 168, 9, 255), true},
		{"4", net.IPv4(192, 168, 255, 255), true},
		{"5", net.IPv4(192, 169, 1, 1), true},
		{"6", net.IPv4(192, 170, 1, 1), false},
		{"7", net.IPv4(1, 1, 1, 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ipNetList.Contains(tt.testIP); got != tt.want {
				t.Errorf("IPNetList.Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIPNetList_New_And_Contains(t *testing.T) {
	raw := `
# comment line
1.0.1.0/24 additional strings should be ignored 
1.1.2.0/23 # comment

1.1.4.0/22 
1.1.8.0/24 
1.0.2.0/23
1.0.8.0/21
1.0.32.0/19
1.1.0.0/24
2001:250::/35
2001:250:2000::/35
2001:250:4000::/34
2001:250:8000::/33
2001:251::/32

2.2.2.2
3.3.3.3
2002:222::1

# issue https://github.com/IrineSistiana/mosdns/issues/76
127.0.0.0/8
`

	ipNetList := NewList()
	err := LoadFromReader(ipNetList, bytes.NewBufferString(raw))
	if err != nil {
		t.Fatal(err)
	}
	ipNetList.Sort()

	tests := []struct {
		name   string
		testIP net.IP
		want   bool
	}{
		{"1", net.IPv4(1, 0, 1, 1), true},
		{"2", net.IPv4(1, 0, 2, 2), true},
		{"3", net.IPv4(1, 1, 1, 1), false},
		{"4", net.IPv4(1, 0, 4, 4), false},
		{"5", net.ParseIP("2001:250:2000::1"), true},
		{"6", net.ParseIP("2002:250:2000::1"), false},
		{"7", net.IPv4(2, 2, 2, 2), true},
		{"8", net.IPv4(2, 2, 2, 3), false},
		{"9", net.IPv4(3, 3, 3, 3), true},
		{"10", net.IPv4(4, 4, 4, 4), false},
		{"11", net.ParseIP("2002:222::1"), true},
		{"12", net.ParseIP("2002:222::2"), false},
		{"https://github.com/IrineSistiana/mosdns/issues/76 1", net.IPv4(127, 0, 0, 1), true},
		{"https://github.com/IrineSistiana/mosdns/issues/76 2", net.IP{127, 0, 0, 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ipNetList.Contains(tt.testIP); got != tt.want {
				t.Errorf("IPNetList.Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cidrMask(t *testing.T) {
	tests := []struct {
		name  string
		n     int
		wantM mask
	}{
		{"0", 0, [2]uint64{0, 0}},
		{"5", 5, [2]uint64{^(maxUint64 >> 5), 0}},
		{"64", 64, [2]uint64{maxUint64, 0}},
		{"120", 120, [2]uint64{maxUint64, ^(maxUint64 >> (120 - 64))}},
		{"128", 128, [2]uint64{maxUint64, maxUint64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotM := cidrMask(tt.n); !reflect.DeepEqual(gotM, tt.wantM) {
				t.Errorf("cidrMask() = %v, want %v", gotM, tt.wantM)
			}
		})
	}
}
