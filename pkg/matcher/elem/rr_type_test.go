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

package elem

import "testing"

func TestIntMatcher_Match(t *testing.T) {
	tests := []struct {
		name string
		m    []int
		args int
		want bool
	}{
		{"nil 1", nil, 1, false},
		{"matched 1", []int{1, 2, 3}, 1, true},
		{"matched 2", []int{1, 2, 3}, 3, true},
		{"not matched 1", []int{1, 2, 3}, 0, false},
		{"not matched 2", []int{1, 2, 3}, 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewIntMatcher(tt.m)
			if got := m.Match(tt.args); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}
