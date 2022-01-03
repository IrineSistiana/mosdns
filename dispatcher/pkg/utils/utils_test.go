//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) or later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package utils

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/miekg/dns"
	"reflect"
	"testing"
)

func TestBoolLogic(t *testing.T) {
	err := errors.New("test err")
	type matcher struct {
		b   bool
		err error
	}
	type args struct {
		fs         []matcher
		logicalAND bool
	}
	tests := []struct {
		name        string
		args        args
		wantMatched bool
		wantErr     bool
	}{
		{"or, ture, no err",
			args{fs: []matcher{
				{false, nil},
				{false, nil},
				{true, nil},
			}, logicalAND: false},
			true, false},
		{"or, true, early stop, no err",
			args{fs: []matcher{
				{true, nil},
				{true, err}, // early stop
				{true, err},
			}, logicalAND: false},
			true, false},
		{"or, false, no err",
			args{fs: []matcher{
				{false, nil},
				{false, nil},
				{false, nil},
			}, logicalAND: false},
			false, false},
		{"or, err",
			args{fs: []matcher{
				{false, nil},
				{false, err},
				{false, nil},
			}, logicalAND: false},
			false, true},

		// match all

		{"all, ture, no err",
			args{fs: []matcher{
				{true, nil},
				{true, nil},
				{true, nil},
			}, logicalAND: true},
			true, false},
		{"all, false, early stop, no err",
			args{fs: []matcher{
				{false, nil},
				{true, err}, // early stop
				{true, err},
			}, logicalAND: true},
			false, false},
		{"all, err",
			args{fs: []matcher{
				{true, nil},
				{false, err},
				{false, err},
			}, logicalAND: true},
			false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion("test.", dns.TypeA)
			qCtx := handler.NewContext(m, nil)
			fs := make([]handler.Matcher, 0)
			for _, b := range tt.args.fs {
				fs = append(fs, &handler.DummyMatcherPlugin{Matched: b.b, WantErr: b.err})
			}
			gotMatched, err := BoolLogic(context.Background(), qCtx, fs, tt.args.logicalAND)
			if (err != nil) != tt.wantErr {
				t.Errorf("BoolLogic() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotMatched != tt.wantMatched {
				t.Errorf("BoolLogic() gotMatched = %v, want %v", gotMatched, tt.wantMatched)
			}
		})
	}
}

func TestSplitLine(t *testing.T) {

	tests := []struct {
		name string
		s    string
		want []string
	}{
		{"blank", "", []string{}},
		{"space", "   ", []string{}},
		{"space", "   a   ", []string{"a"}},
		{"space", "   a", []string{"a"}},
		{"split", " 1 22 333 4444  ", []string{"1", "22", "333", "4444"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SplitLine(tt.s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

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

func TestIsIPAddr(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"host", "dns.google", false},
		{"host:port", "dns.google:53", false},
		{"ip", "8.8.8.8", true},
		{"ip:port", "8.8.8.8:53", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIPAddr(tt.s); got != tt.want {
				t.Errorf("IsIPAddr() = %v, want %v", got, tt.want)
			}
		})
	}
}
