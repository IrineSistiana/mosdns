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

package domain

import (
	"reflect"
	"testing"
)

func TestParseV2Suffix(t *testing.T) {
	tests := []struct {
		name string
		args string
		want []*V2DomainPicker
	}{
		{"1", "test@a1@a2,,", []*V2DomainPicker{{tag: "test", attrs: attrMap([]string{"a1", "a2"})}}},
		{"1", ",test@a1,,test@a1", []*V2DomainPicker{
			{tag: "test", attrs: attrMap([]string{"a1"})},
			{tag: "test", attrs: attrMap([]string{"a1"})},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseV2Suffix(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseV2Suffix() = %v, want %v", got, tt.want)
			}
		})
	}
}
