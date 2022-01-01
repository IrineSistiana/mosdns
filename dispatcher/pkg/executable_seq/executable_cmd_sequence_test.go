package executable_seq

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
	"testing"
)

func Test_ECS(t *testing.T) {
	handler.PurgePluginRegister()
	defer handler.PurgePluginRegister()

	mErr := errors.New("mErr")
	eErr := errors.New("eErr")
	target := new(dns.Msg)
	target.Id = dns.Id()

	var tests = []struct {
		name       string
		yamlStr    string
		wantTarget bool
		wantErr    error
	}{

		{name: "test ! prefix", yamlStr: `
exec:
- if: ["!matched", not_matched] # test ! prefix, not matched
  exec: exec_err
- if: ["!not_matched"]
  exec: exec_target  # reached here
`,
			wantTarget: true, wantErr: nil},

		{name: "test matcher short circuit", yamlStr: `
exec:
- if: [matched, match_err] # logic short circuit, match_err won't run
  exec: exec_target
`,
			wantTarget: true, wantErr: nil},

		{name: "test muti_if", yamlStr: `
exec:
- if: [matched]
  exec: [exec,exec,exec]
- if: [matched]
  exec: exec_target
`,
			wantTarget: true, wantErr: nil},

		{name: "test if else_exec", yamlStr: `
exec:
- if: ['!matched']
  exec: exec_err
  else_exec: exec_target
`,
			wantTarget: true, wantErr: nil},

		{name: "test multi if else_exec", yamlStr: `
exec:
- if: ['!matched']
  exec: [exec_err]
  else_exec: [exec]
- if: ['!matched']
  exec: [exec_err]
  else_exec: [exec_target]
`,
			wantTarget: true, wantErr: nil},

		{name: "test nested if", yamlStr: `
exec:
- if: [matched]
  exec: 
  - exec
  - exec
  - if: [matched]
    exec: exec_target
`,
			wantTarget: true, wantErr: nil},

		{name: "test if_and", yamlStr: `
exec:
- if_and: [matched, not_matched] # not matched
  exec: exec_err
- if_and: [matched, matched] # matched
  exec: exec_target
`,
			wantTarget: true, wantErr: nil},

		{name: "test if err", yamlStr: `
exec:
- if: [not_matched, match_err] # err
  exec: exec
`,
			wantTarget: false, wantErr: mErr},

		{name: "test if_and err", yamlStr: `
exec:
- if_and: [matched, match_err] # err
  exec: exec
`,
			wantTarget: false, wantErr: mErr},

		{name: "test exec err", yamlStr: `
exec:
- exec
- exec_err
`,
			wantTarget: false, wantErr: eErr},

		{name: "test exec err in if branch", yamlStr: `
exec:
- if: [matched] 
  exec: 
  - exec
  - exec_err
`,
			wantTarget: false, wantErr: eErr},

		{name: "test return in main sequence", yamlStr: `
exec:
- exec
- exec_skip
- exec_err 	# skipped, should not reach here.
`,
			wantTarget: false, wantErr: nil},

		{name: "test early return in if branch", yamlStr: `
exec:
- if: [matched] 
  exec: 
    - exec_skip
    - exec_err # skipped, should not reach here.
- exec_err
`,
			wantTarget: false, wantErr: nil},
	}

	// not_matched
	handler.MustRegPlugin(&handler.DummyMatcherPlugin{
		BP:      handler.NewBP("not_matched", ""),
		Matched: false,
		WantErr: nil,
	})

	// do something
	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:      handler.NewBP("exec", ""),
		WantErr: nil,
	})

	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:      handler.NewBP("exec_target", ""),
		WantR:   target,
		WantErr: nil,
	})

	// do something and skip the following sequence
	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:       handler.NewBP("exec_skip", ""),
		WantSkip: true,
	})

	// matched
	handler.MustRegPlugin(&handler.DummyMatcherPlugin{
		BP:      handler.NewBP("matched", ""),
		Matched: true,
		WantErr: nil,
	})

	// plugins should return an err.
	handler.MustRegPlugin(&handler.DummyMatcherPlugin{
		BP:      handler.NewBP("match_err", ""),
		Matched: false,
		WantErr: mErr,
	})

	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:      handler.NewBP("exec_err", ""),
		WantErr: eErr,
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := make(map[string]interface{}, 0)
			err := yaml.Unmarshal([]byte(tt.yamlStr), args)
			if err != nil {
				t.Fatal(err)
			}

			ecs, err := ParseExecutableNode(args["exec"], nil)
			if err != nil {
				t.Fatal(err)
			}

			qCtx := handler.NewContext(new(dns.Msg), nil)
			err = handler.ExecChainNode(context.Background(), qCtx, ecs)
			if (err != nil || tt.wantErr != nil) && !errors.Is(err, tt.wantErr) {
				t.Errorf("Exec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var gotTarget = qCtx.R()
			if tt.wantTarget && gotTarget.Id != target.Id {
				t.Errorf("Exec() gotTarget = %d, want %d", gotTarget.Id, target.Id)
			}
		})
	}
}

func Test_LoadBalance(t *testing.T) {
	handler.PurgePluginRegister()
	defer handler.PurgePluginRegister()

	eErr := errors.New("eErr")
	target := new(dns.Msg)
	target.Id = dns.Id()

	type want struct {
		target bool
		err    error
	}

	var tests = []struct {
		name    string
		yamlStr string
		want    []want
	}{

		{name: "test round robin", yamlStr: `
exec:
- load_balance:
  - - exec_err
  - - exec
  - - exec_target
`,
			// LoadBalance executes the branch#2 first.
			want: []want{
				{false, nil},
				{true, nil},
				{false, eErr},
			}},

		{name: "test node connection", yamlStr: `
exec:
- load_balance:
  - - exec_err
  - - exec_skip
  - - exec
- exec_target
`,
			want: []want{
				{false, nil},
				{true, nil},
				{false, eErr},
			}},
	}

	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:      handler.NewBP("exec", ""),
		WantErr: nil,
	})

	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:      handler.NewBP("exec_target", ""),
		WantR:   target,
		WantErr: nil,
	})

	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:       handler.NewBP("exec_skip", ""),
		WantSkip: true,
	})

	handler.MustRegPlugin(&handler.DummyExecutablePlugin{
		BP:      handler.NewBP("exec_err", ""),
		WantErr: eErr,
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := make(map[string]interface{}, 0)
			err := yaml.Unmarshal([]byte(tt.yamlStr), args)
			if err != nil {
				t.Fatal(err)
			}

			ecs, err := ParseExecutableNode(args["exec"], nil)
			if err != nil {
				t.Fatal(err)
			}

			for r := 0; r < 3; r++ {
				for _, want := range tt.want {
					qCtx := handler.NewContext(new(dns.Msg), nil)
					err = handler.ExecChainNode(context.Background(), qCtx, ecs)
					if (err != nil || want.err != nil) && !errors.Is(err, want.err) {
						t.Errorf("Exec() error = %v, wantErr %v", err, want.err)
						return
					}

					var gotTarget = qCtx.R()
					if want.target && gotTarget.Id != target.Id {
						t.Errorf("Exec() gotTarget = %d, want %d", gotTarget.Id, target.Id)
					}
				}
			}

		})
	}
}
