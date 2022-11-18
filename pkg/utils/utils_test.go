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

package utils

import (
	"reflect"
	"testing"
)

func TestSplitString2(t *testing.T) {
	type args struct {
		s      string
		symbol string
	}
	tests := []struct {
		name   string
		args   args
		wantS1 string
		wantS2 string
		wantOk bool
	}{
		{"blank", args{"", ""}, "", "", true},
		{"blank", args{"///", ""}, "", "///", true},
		{"split", args{"///", "/"}, "", "//", true},
		{"split", args{"--/", "/"}, "--", "", true},
		{"split", args{"https://***.***.***", "://"}, "https", "***.***.***", true},
		{"split", args{"://***.***.***", "://"}, "", "***.***.***", true},
		{"split", args{"https://", "://"}, "https", "", true},
		{"split", args{"--/", "*"}, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotS1, gotS2, gotOk := SplitString2(tt.args.s, tt.args.symbol)
			if gotS1 != tt.wantS1 {
				t.Errorf("SplitString2() gotS1 = %v, want %v", gotS1, tt.wantS1)
			}
			if gotS2 != tt.wantS2 {
				t.Errorf("SplitString2() gotS2 = %v, want %v", gotS2, tt.wantS2)
			}
			if gotOk != tt.wantOk {
				t.Errorf("SplitString2() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func TestRemoveComment(t *testing.T) {
	type args struct {
		s      string
		symbol string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "empty", args: args{s: "", symbol: ""}, want: ""},
		{name: "empty symbol", args: args{s: "12345", symbol: ""}, want: ""},
		{name: "empty string", args: args{s: "", symbol: "#"}, want: ""},
		{name: "remove 1", args: args{s: "123/456", symbol: "/"}, want: "123"},
		{name: "remove 2", args: args{s: "123//456", symbol: "//"}, want: "123"},
		{name: "remove 3", args: args{s: "123/*/456", symbol: "//"}, want: "123/*/456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveComment(tt.args.s, tt.args.symbol); got != tt.want {
				t.Errorf("RemoveComment() = %v, want %v", got, tt.want)
			}
		})
	}
}

type TestArgsStruct struct {
	A string `yaml:"1"`
	B []int  `yaml:"2"`
}

func Test_WeakDecode(t *testing.T) {
	testObj := new(TestArgsStruct)
	testArgs := map[string]interface{}{
		"1": "test",
		"2": []int{1, 2, 3},
	}
	wantObj := &TestArgsStruct{
		A: "test",
		B: []int{1, 2, 3},
	}

	err := WeakDecode(testArgs, testObj)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(testObj, wantObj) {
		t.Fatalf("args decode failed, want %v, got %v", wantObj, testObj)
	}
}

func Test_WeakDecode2(t *testing.T) {
	testObj := new([]byte)
	args := []any{"1", 2, 3}
	err := WeakDecode(args, testObj)
	if err != nil {
		t.Fatal(err)
	}
}
