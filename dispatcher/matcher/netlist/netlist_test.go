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

var (
	rawList = `
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

192.168.0.0/16
192.168.1.1/24
192.168.9.24/24
192.168.3.0/24
192.169.0.0/16
`
)

func TestIPNetList_New_And_Contains(t *testing.T) {
	ipNetList, err := NewListFromReader(bytes.NewBufferString(rawList))
	if err != nil {
		t.Fatal(err)
	}

	if ipNetList.Len() != 18 {
		t.Fatalf("unexpected length %d", ipNetList.Len())
	}

	type args struct {
		ip net.IP
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"1", args{net.IPv4(1, 0, 1, 1)}, true},
		{"2", args{net.IPv4(1, 0, 2, 2)}, true},
		{"3", args{net.IPv4(1, 1, 1, 1)}, false},
		{"4", args{net.IPv4(1, 0, 4, 4)}, false},
		{"5", args{net.ParseIP("2001:250:2000::1")}, true},
		{"6", args{net.ParseIP("2002:250:2000::1")}, false},
		{"7", args{net.IPv4(2, 2, 2, 2)}, true},
		{"8", args{net.IPv4(2, 2, 2, 3)}, false},
		{"9", args{net.IPv4(3, 3, 3, 3)}, true},
		{"10", args{net.IPv4(4, 4, 4, 4)}, false},
		{"11", args{net.ParseIP("2002:222::1")}, true},
		{"12", args{net.ParseIP("2002:222::2")}, false},
		{"13", args{net.IPv4(192, 168, 4, 4)}, true},
		{"14", args{net.IPv4(192, 168, 255, 255)}, true},
		{"15", args{net.IPv4(192, 169, 4, 4)}, true},
		{"14", args{net.IPv4(192, 170, 4, 4)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ipNetList.Contains(tt.args.ip); got != tt.want {
				t.Errorf("IPNetList.Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cidrMask(t *testing.T) {
	tests := []struct {
		name  string
		n     uint
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
