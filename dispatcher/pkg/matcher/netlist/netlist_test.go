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
	"math/bits"
	"net"
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
1.0.0.0/24 additional strings should be ignored 
2.0.0.0/23 # comment
3.0.0.0

2000:0000::/32
2000:2000::1

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
		{"", net.ParseIP("1.0.0.0"), true},
		{"", net.ParseIP("1.0.0.1"), true},
		{"", net.ParseIP("1.0.1.0"), false},
		{"", net.ParseIP("2.0.0.0"), true},
		{"", net.ParseIP("2.0.1.255"), true},
		{"", net.ParseIP("2.0.2.0"), false},
		{"", net.ParseIP("3.0.0.0"), true},
		{"", net.ParseIP("2000:0000::"), true},
		{"", net.ParseIP("2000:0000::1"), true},
		{"", net.ParseIP("2000:0000:1::"), true},
		{"", net.ParseIP("2000:0001::"), false},
		{"", net.ParseIP("2000:2000::1"), true},
		{"https://github.com/IrineSistiana/mosdns/issues/76", net.IPv4(127, 0, 0, 1), true},
		{"https://github.com/IrineSistiana/mosdns/issues/76", net.IP{127, 0, 0, 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ipNetList.Contains(tt.testIP); got != tt.want {
				t.Errorf("IPNetList.Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_initMasks(t *testing.T) {
	for i := 0; i < 129; i++ {
		m := masks[i]
		ones := 0
		zeros := 0
		for j := 0; j < 2; j++ {
			ones += bits.OnesCount64(m[j])
			zeros += bits.TrailingZeros64(m[j])
		}
		if ones != i || zeros != (128-ones) {
			t.Fatalf("%v is not a /%d mask", m, i)
		}
	}
}
