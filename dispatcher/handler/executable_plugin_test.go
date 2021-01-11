package handler

import (
	"context"
	"errors"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"testing"
	"time"
)

func Test_ECS(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

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
	MustRegPlugin(&DummyMatcherPlugin{
		BP:      NewBP("not_matched", ""),
		Matched: false,
		WantErr: nil,
	}, true)

	// do something
	MustRegPlugin(&DummyExecutablePlugin{
		BP:      NewBP("exec", ""),
		WantErr: nil,
	}, true)

	// do something and skip the following sequence
	MustRegPlugin(&DummyESExecutablePlugin{
		BP:       NewBP("exec_skip", ""),
		WantSkip: true,
	}, true)

	// matched
	MustRegPlugin(&DummyMatcherPlugin{
		BP:      NewBP("matched", ""),
		Matched: true,
		WantErr: nil,
	}, true)

	// plugins should return an err.
	MustRegPlugin(&DummyMatcherPlugin{
		BP:      NewBP("match_err", ""),
		Matched: false,
		WantErr: mErr,
	}, true)

	MustRegPlugin(&DummyExecutablePlugin{
		BP:      NewBP("exec_err", ""),
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

			gotNext, gotEarlyStop, err := ecs.ExecCmd(context.Background(), NewContext(new(dns.Msg), nil), zap.NewNop())
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
	PurgePluginRegister()
	defer PurgePluginRegister()

	r1 := new(dns.Msg)
	r2 := new(dns.Msg)
	p1 := &DummyExecutablePlugin{BP: NewBP("p1", "")}
	p2 := &DummyExecutablePlugin{BP: NewBP("p2", "")}
	MustRegPlugin(p1, true)
	MustRegPlugin(p2, true)

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

	parallelECS, err := ParseParallelECS([][]interface{}{{"p1"}, {"p2"}})
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

			qCtx := NewContext(new(dns.Msg), nil)
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

func Test_FallbackECS(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	r1 := new(dns.Msg)
	r2 := new(dns.Msg)
	p1 := &DummyExecutablePlugin{BP: NewBP("p1", "")}
	p2 := &DummyExecutablePlugin{BP: NewBP("p2", ""), WantR: r2}
	MustRegPlugin(p1, true)
	MustRegPlugin(p2, true)
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

	fallbackECS, err := ParseFallbackECS([]interface{}{"p1"}, []interface{}{"p2"}, 2, 3)
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

			qCtx := NewContext(new(dns.Msg), nil)
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
