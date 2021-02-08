package executable_seq

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"testing"
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
		{name: "test empty input", yamlStr: `
exec:
`,
			wantNext: "", wantErr: nil},
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
			in, _ := args["exec"].([]interface{})
			ecs, err := ParseExecutableCmdSequence(in)
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
