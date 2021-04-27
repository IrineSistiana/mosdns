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

package executable_seq

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"testing"
)

func Test_ParallelECS(t *testing.T) {
	handler.PurgePluginRegister()
	defer handler.PurgePluginRegister()

	r1 := new(dns.Msg)
	r2 := new(dns.Msg)

	er := errors.New("")
	tests := []struct {
		name    string
		r1      *dns.Msg
		e1      error
		r2      *dns.Msg
		e2      error
		wantR   *dns.Msg
		wantErr bool
	}{
		{"failed #1", nil, er, nil, er, nil, true},
		{"failed #2", nil, nil, nil, nil, nil, true},
		{"p1 response #1", r1, nil, nil, nil, r1, false},
		{"p1 response #2", r1, nil, nil, er, r1, false},
		{"p2 response #1", nil, nil, r2, nil, r2, false},
		{"p2 response #2", nil, er, r2, nil, r2, false},
	}

	parallelECS, err := ParseParallelECS(&ParallelECSConfig{
		Parallel: []interface{}{"p1", "p2"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p1 := &handler.DummyExecutablePlugin{
				BP:      handler.NewBP("p1", ""),
				Sleep:   0,
				WantR:   tt.r1,
				WantErr: tt.e1,
			}
			p2 := &handler.DummyExecutablePlugin{
				BP:      handler.NewBP("p2", ""),
				Sleep:   0,
				WantR:   tt.r2,
				WantErr: tt.e2,
			}
			handler.MustRegPlugin(p1, false)
			handler.MustRegPlugin(p2, false)

			qCtx := handler.NewContext(new(dns.Msg), nil)
			err := parallelECS.execCmd(ctx, qCtx, zap.NewNop())
			if tt.wantErr != (err != nil) {
				t.Fatalf("execCmd() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantR != qCtx.R() {
				t.Fatalf("execCmd() qCtx.R() = %p, wantR %p", qCtx.R(), tt.wantR)
			}
		})
	}
}
