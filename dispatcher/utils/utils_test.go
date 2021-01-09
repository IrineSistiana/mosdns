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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
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
