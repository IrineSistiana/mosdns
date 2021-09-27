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
	"testing"
	"time"
)

func Test_FallbackECS_fallback(t *testing.T) {
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
		{"failed 0", nil, er, r2, nil, nil, true}, // warm up
		{"failed 1", nil, er, r2, nil, nil, true},
		{"failed 2", nil, er, r2, nil, r2, false}, // trigger fallback
		{"failed 3", nil, nil, r2, nil, r2, false},
		{"failed 3", r1, nil, nil, nil, r1, false}, // primary success
		{"success 1 failed 2", r1, nil, nil, nil, r1, false},
		{"success 2 failed 1", nil, er, nil, nil, nil, true}, // end of fallback, but primary returns an err again
		{"success 1 failed 2", nil, er, nil, er, nil, true},  // no response
	}
	conf := &FallbackConfig{
		Primary:       []interface{}{"p1"},
		Secondary:     []interface{}{"p2"},
		StatLength:    2,
		Threshold:     3,
		FastFallback:  0,
		AlwaysStandby: false,
	}

	fallbackECS, err := ParseFallbackNode(conf, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p1 := &handler.DummyExecutablePlugin{
				BP:        handler.NewBP("p1", ""),
				WantSleep: 0,
				WantR:     tt.r1,
				WantErr:   tt.e1,
			}
			p2 := &handler.DummyExecutablePlugin{
				BP:        handler.NewBP("p2", ""),
				WantSleep: 0,
				WantR:     tt.r2,
				WantErr:   tt.e2,
			}

			handler.MustRegPlugin(p1, false)
			handler.MustRegPlugin(p2, false)
			qCtx := handler.NewContext(new(dns.Msg), nil)
			err := handler.ExecChainNode(ctx, qCtx, handler.WarpExecutable(fallbackECS))
			if tt.wantErr != (err != nil) {
				t.Fatalf("execCmd() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantR != qCtx.R() {
				t.Fatalf("execCmd() qCtx.R() = %p, wantR %p", qCtx.R(), tt.wantR)
			}

			time.Sleep(time.Millisecond * 20) // wait for statusTracker.update()
		})
	}
}

func Test_FallbackECS_fast_fallback(t *testing.T) {
	handler.PurgePluginRegister()
	defer handler.PurgePluginRegister()

	r1 := new(dns.Msg)
	r2 := new(dns.Msg)

	er := errors.New("")

	tests := []struct {
		name          string
		r1            *dns.Msg
		e1            error
		l1            int
		r2            *dns.Msg
		e2            error
		l2            int
		alwaysStandby bool
		wantLatency   int
		wantR         *dns.Msg
		wantErr       bool
	}{
		{"p succeed", r1, nil, 50, r2, nil, 0, false, 70, r1, false},
		{"p failed", nil, er, 0, r2, nil, 0, false, 20, r2, false},
		{"p timeout", r1, nil, 200, r2, nil, 0, false, 120, r2, false},
		{"p timeout, s failed", r1, nil, 200, nil, er, 0, false, 220, r1, false},
		{"all timeout", r1, nil, 400, r2, nil, 400, false, 320, nil, true},
		{"always standby p succeed", r1, nil, 50, r2, nil, 0, true, 70, r1, false},
		{"always standby p failed", nil, er, 50, r2, nil, 50, true, 70, r2, false},
		{"always standby p timeout", r1, nil, 200, r2, nil, 50, true, 120, r2, false},
		{"always standby p timeout, s failed", r1, nil, 200, nil, er, 0, true, 220, r1, false},
		{"always standby all timeout", r1, nil, 400, r2, nil, 400, true, 320, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := &FallbackConfig{
				Primary:       []interface{}{"p1"},
				Secondary:     []interface{}{"p2"},
				StatLength:    0, // never trigger the normal fallback mode
				Threshold:     0,
				FastFallback:  100,
				AlwaysStandby: tt.alwaysStandby,
			}

			fallbackECS, err := ParseFallbackNode(conf, nil)
			if err != nil {
				t.Fatal(err)
			}
			p1 := &handler.DummyExecutablePlugin{
				BP:        handler.NewBP("p1", ""),
				WantSleep: time.Duration(tt.l1) * time.Millisecond,
				WantR:     tt.r1,
				WantErr:   tt.e1,
			}
			p2 := &handler.DummyExecutablePlugin{
				BP:        handler.NewBP("p2", ""),
				WantSleep: time.Duration(tt.l2) * time.Millisecond,
				WantR:     tt.r2,
				WantErr:   tt.e2,
			}
			handler.MustRegPlugin(p1, false)
			handler.MustRegPlugin(p2, false)

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()

			start := time.Now()
			qCtx := handler.NewContext(new(dns.Msg), nil)
			err = handler.ExecChainNode(ctx, qCtx, handler.WarpExecutable(fallbackECS))
			if time.Since(start) > time.Millisecond*time.Duration(tt.wantLatency) {
				t.Fatalf("execCmd() timeout: latency = %vms, want = %vms", time.Since(start).Milliseconds(), tt.wantLatency)
			}
			if tt.wantErr != (err != nil) {
				t.Fatalf("execCmd() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantR != qCtx.R() {
				t.Fatalf("execCmd() qCtx.R() = %p, wantR %p", qCtx.R(), tt.wantR)
			}

		})
	}
}

func Test_statusTracker(t *testing.T) {
	tests := []struct {
		name string
		in   []uint8

		wantGood bool
	}{
		{"start0", nil, true},
		{"start1", []uint8{0}, true},
		{"start2", []uint8{0, 0}, true},
		{"start3", []uint8{0, 0, 0}, true},
		{"start4", []uint8{1, 1}, true},
		{"start5", []uint8{1, 1, 1}, false},
		{"start6", []uint8{1, 1, 1, 0}, false},
		{"run1", []uint8{1, 1, 1, 0, 0}, true},
		{"run2", []uint8{1, 1, 1, 1, 1, 1, 0, 0}, true},
		{"run3", []uint8{0, 0, 0, 0, 0, 1, 1, 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newStatusTracker(3, 4)
			for _, s := range tt.in {
				st.update(s)
			}

			if st.good() != tt.wantGood {
				t.Fatal()
			}
		})
	}
}
