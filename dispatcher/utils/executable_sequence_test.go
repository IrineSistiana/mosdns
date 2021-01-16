package utils

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"testing"
	"time"
)

func Test_ECS(t *testing.T) {
	handler.PurgePluginRegister()
	defer handler.PurgePluginRegister()

	mErr := errors.New("mErr")
	eErr := errors.New("eErr")

	var tests = []struct {
		name     string
		yamlStr  string
		wantNext string
		wantES   bool
		wantErr  error
	}{
		{name: "test empty end", yamlStr: `
exec:
- if: ["!matched",not_matched] # not matched
  exec: [exec_err]
  goto: goto`,
			wantNext: "", wantErr: nil},

		{name: "test if_and", yamlStr: `
exec:
- if_and: [matched, not_matched] # not matched
  goto: goto1
- if_and: [matched, not_matched, match_err] # not matched, early stop, no err
  goto: goto2
- if_and: [matched, matched, matched] # matched
  goto: goto3
`,
			wantNext: "goto3", wantErr: nil},

		{name: "test if_and err", yamlStr: `
exec:
- if_and: [matched, match_err] # err
  goto: goto1
`,
			wantNext: "", wantErr: mErr},

		{name: "test if", yamlStr: `
exec:
- if: ["!matched", not_matched] # test ! prefix, not matched
  goto: goto1
- if: [matched, match_err] # matched, early stop, no err
  exec:
  - if: ["!not_matched", not_matched] # matched
    goto: goto2 						# reached here
  goto: goto3
`,
			wantNext: "goto2", wantErr: nil},

		{name: "test if err", yamlStr: `
exec:
- if: [not_matched, match_err] # err
  goto: goto1
`,
			wantNext: "", wantErr: mErr},

		{name: "test exec err", yamlStr: `
exec:
- if: [matched] 
  exec: exec_err
  goto: goto1
`,
			wantNext: "", wantErr: eErr},

		{name: "test early return in main sequence", yamlStr: `
exec:
- exec
- exec_skip
- exec_err 	# skipped, should not reach here.
`,
			wantNext: "", wantES: true, wantErr: nil},

		{name: "test early return in if branch", yamlStr: `
exec:
- if: [matched] 
  exec: 
    - exec_skip
  goto: goto1 # skipped, should not reach here.
`,
			wantNext: "", wantES: true, wantErr: nil},
	}

	// not_matched
	handler.MustRegPlugin(&handler.DummyMatcherPlugin{
		BP:      handler.NewBP("not_matched", ""),
		Matched: false,
		WantErr: nil,
	}, true)

	// do something
	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:      handler.NewBP("exec", ""),
		WantErr: nil,
	}, true)

	// do something and skip the following sequence
	handler.MustRegPlugin(&handler.DummyESExecutablePlugin{
		BP:       handler.NewBP("exec_skip", ""),
		WantSkip: true,
	}, true)

	// matched
	handler.MustRegPlugin(&handler.DummyMatcherPlugin{
		BP:      handler.NewBP("matched", ""),
		Matched: true,
		WantErr: nil,
	}, true)

	// plugins should return an err.
	handler.MustRegPlugin(&handler.DummyMatcherPlugin{
		BP:      handler.NewBP("match_err", ""),
		Matched: false,
		WantErr: mErr,
	}, true)

	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:      handler.NewBP("exec_err", ""),
		WantErr: eErr,
	}, true)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := make(map[string]interface{}, 0)
			err := yaml.Unmarshal([]byte(tt.yamlStr), args)
			if err != nil {
				t.Fatal(err)
			}
			ecs, err := ParseExecutableCmdSequence(args["exec"].([]interface{}))
			if err != nil {
				t.Fatal(err)
			}

			gotNext, gotEarlyStop, err := ecs.ExecCmd(context.Background(), handler.NewContext(new(dns.Msg), nil), zap.NewNop())
			if (err != nil || tt.wantErr != nil) && !errors.Is(err, tt.wantErr) {
				t.Errorf("ExecCmd() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotNext != tt.wantNext {
				t.Errorf("ExecCmd() gotNext = %v, want %v", gotNext, tt.wantNext)
			}

			if gotEarlyStop != tt.wantES {
				t.Errorf("ExecCmd() gotEarlyStop = %v, want %v", gotEarlyStop, tt.wantES)
			}
		})
	}
}

func Test_ParallelECS(t *testing.T) {
	handler.PurgePluginRegister()
	defer handler.PurgePluginRegister()

	r1 := new(dns.Msg)
	r2 := new(dns.Msg)
	p1 := &handler.DummyExecutablePlugin{BP: handler.NewBP("p1", "")}
	p2 := &handler.DummyExecutablePlugin{BP: handler.NewBP("p2", "")}
	handler.MustRegPlugin(p1, true)
	handler.MustRegPlugin(p2, true)

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
		Parallel: [][]interface{}{{"p1"}, {"p2"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p1.WantR = tt.r1
			p1.WantErr = tt.e1
			p2.WantR = tt.r2
			p2.WantErr = tt.e2

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

func Test_FallbackECS_fallback(t *testing.T) {
	handler.PurgePluginRegister()
	defer handler.PurgePluginRegister()

	r1 := new(dns.Msg)
	r2 := new(dns.Msg)
	p1 := &handler.DummyExecutablePlugin{BP: handler.NewBP("p1", "")}
	p2 := &handler.DummyExecutablePlugin{BP: handler.NewBP("p2", "")}
	handler.MustRegPlugin(p1, true)
	handler.MustRegPlugin(p2, true)
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

	fallbackECS, err := ParseFallbackECS(conf)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p1.WantR = tt.r1
			p1.WantErr = tt.e1
			p2.WantR = tt.r2
			p2.WantErr = tt.e2

			qCtx := handler.NewContext(new(dns.Msg), nil)
			err := fallbackECS.execCmd(ctx, qCtx, zap.NewNop())
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
	p1 := &handler.DummyExecutablePlugin{BP: handler.NewBP("p1", "")}
	p2 := &handler.DummyExecutablePlugin{BP: handler.NewBP("p2", "")}
	handler.MustRegPlugin(p1, true)
	handler.MustRegPlugin(p2, true)
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
				StatLength:    99999, // never trigger the fallback mode
				Threshold:     99999,
				FastFallback:  100,
				AlwaysStandby: tt.alwaysStandby,
			}

			fallbackECS, err := ParseFallbackECS(conf)
			if err != nil {
				t.Fatal(err)
			}

			p1.WantR = tt.r1
			p1.WantErr = tt.e1
			p1.Sleep = time.Duration(tt.l1) * time.Millisecond
			p2.WantR = tt.r2
			p2.WantErr = tt.e2
			p2.Sleep = time.Duration(tt.l2) * time.Millisecond

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()

			start := time.Now()
			qCtx := handler.NewContext(new(dns.Msg), nil)
			err = fallbackECS.execCmd(ctx, qCtx, zap.NewNop())
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
