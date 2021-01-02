package handler

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"gopkg.in/yaml.v3"
	"testing"
)

func Test_switchPlugin_Do(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	mErr := errors.New("mErr")
	eErr := errors.New("eErr")

	type args struct {
		executable ExecutableCmd
	}

	var tests = []struct {
		name     string
		yamlStr  string
		wantNext string
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
	}

	mustSuccess := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	// not_matched
	mustSuccess(RegPlugin(WrapMatcherPlugin("not_matched", "",
		&DummyMatcher{Matched: false, WantErr: nil},
	)))

	// do something
	mustSuccess(RegPlugin(WrapExecutablePlugin("exec", "",
		&DummyExecutable{WantErr: nil},
	)))

	// matched
	mustSuccess(RegPlugin(WrapMatcherPlugin("matched", "",
		&DummyMatcher{Matched: true, WantErr: nil},
	)))

	// plugins should return an err.
	mustSuccess(RegPlugin(WrapMatcherPlugin("match_err", "",
		&DummyMatcher{Matched: false, WantErr: mErr},
	)))
	mustSuccess(RegPlugin(WrapExecutablePlugin(string("exec_err"), "",
		&DummyExecutable{WantErr: eErr},
	)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := make(map[string]interface{}, 0)
			err := yaml.Unmarshal([]byte(tt.yamlStr), args)
			if err != nil {
				t.Fatal(err)
			}
			ecs := NewExecutableCmdSequence()
			err = ecs.Parse(args["exec"].([]interface{}))
			if err != nil {
				t.Fatal(err)
			}

			gotNext, err := ecs.ExecCmd(context.Background(), nil, mlog.NewPluginLogger("test"))
			if (err != nil || tt.wantErr != nil) && !errors.Is(err, tt.wantErr) {
				t.Errorf("Do() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotNext != tt.wantNext {
				t.Errorf("Do() gotNext = %v, want %v", gotNext, tt.wantNext)
			}
		})
	}
}
