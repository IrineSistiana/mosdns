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

func TestDomainScanner(t *testing.T) {
	tests := []struct {
		name           string
		fqdn           string
		wantOffsets    []int
		wantLabels     []string
		wantSubDomains []string
	}{
		{"empty", "", []int{}, []string{}, []string{}},
		{"root", ".", []int{}, []string{}, []string{}},
		{"non fqdn", "a.2", []int{2, 0}, []string{"2", "a"}, []string{"2", "a.2"}},
		{"domain", "1.2.3.", []int{4, 2, 0}, []string{"3", "2", "1"}, []string{"3", "2.3", "1.2.3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewUnifiedDomainScanner(tt.fqdn)
			gotOffsets := make([]int, 0)
			for s.Scan() {
				gotOffsets = append(gotOffsets, s.PrevLabelOffset())
			}
			if !reflect.DeepEqual(gotOffsets, tt.wantOffsets) {
				t.Errorf("PrevLabelOffset() = %v, want %v", gotOffsets, tt.wantOffsets)
			}

			s = NewUnifiedDomainScanner(tt.fqdn)
			gotLabels := make([]string, 0)
			for s.Scan() {
				pl, _ := s.PrevLabel()
				gotLabels = append(gotLabels, pl)
			}
			if !reflect.DeepEqual(gotLabels, tt.wantLabels) {
				t.Errorf("PrevLabel() = %v, want %v", gotLabels, tt.wantLabels)
			}

			s = NewUnifiedDomainScanner(tt.fqdn)
			gotSubDomains := make([]string, 0)
			for s.Scan() {
				sd, _ := s.PrevSubDomain()
				gotSubDomains = append(gotSubDomains, sd)
			}
			if !reflect.DeepEqual(gotSubDomains, tt.wantSubDomains) {
				t.Errorf("PrevLabel() = %v, want %v", gotSubDomains, tt.wantSubDomains)
			}
		})
	}
}
