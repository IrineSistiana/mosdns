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

package sequence

import (
	"reflect"
	"testing"
)

func Test_parseExec(t *testing.T) {

	tests := []struct {
		name     string
		args     string
		wantTag  string
		wantTyp  string
		wantArgs string
	}{
		{"", " $t1   a 1  ", "t1", "", "a 1"},
		{"", " typ   a 1  ", "", "typ", "a 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTag, gotTyp, gotArgs := parseExec(tt.args)
			if gotTag != tt.wantTag {
				t.Errorf("parseExec() gotTag = %v, want %v", gotTag, tt.wantTag)
			}
			if gotTyp != tt.wantTyp {
				t.Errorf("parseExec() gotTyp = %v, want %v", gotTyp, tt.wantTyp)
			}
			if gotArgs != tt.wantArgs {
				t.Errorf("parseExec() gotArgs = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func Test_parseMatch(t *testing.T) {
	tests := []struct {
		name string
		args string
		want MatchConfig
	}{
		{"", " $m1  a 1 ", MatchConfig{
			Tag:     "m1",
			Type:    "",
			Args:    "a 1",
			Reverse: false,
		}},
		{"", " ! typ  a 1 ", MatchConfig{
			Tag:     "",
			Type:    "typ",
			Args:    "a 1",
			Reverse: true,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseMatch(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
