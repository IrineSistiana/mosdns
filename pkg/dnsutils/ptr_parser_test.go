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

package dnsutils

import (
	"net/netip"
	"reflect"
	"testing"
)

func Test_reverse4(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    netip.Addr
		wantErr bool
	}{
		{"v4", "4.4.8.8", netip.MustParseAddr("8.8.4.4"), false},
		{"v4_with_prefix", "prefix.4.4.8.8", netip.MustParseAddr("8.8.4.4"), false},
		{"invalid_format", "123114123", netip.Addr{}, true},
		{"invalid_format", "12..311..4123..", netip.Addr{}, true},
		{"invalid_format", "...", netip.Addr{}, true},
		{"short_length", "4.8.8", netip.Addr{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reverse4(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("reverse4() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reverse4() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_reverse6(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    netip.Addr
		wantErr bool
	}{
		{"v6", "b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2", netip.MustParseAddr("2001:db8::567:89ab"), false},
		{"v6_with_prefix", "prefix.b.a.9.8.7.6.5.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2", netip.MustParseAddr("2001:db8::567:89ab"), false},
		{"invalid_format", "123114123", netip.Addr{}, true},
		{"invalid_format", "..123...", netip.Addr{}, true},
		{"short_length", "0.0.0.0.0.0.8.b.d.0.1.0.0.2", netip.Addr{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reverse6(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("reverse6() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reverse4() got = %v, want %v", got, tt.want)
			}
		})
	}
}
