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

package executable_seq

import (
	"context"
	"errors"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"testing"
)

func Test_ECS(t *testing.T) {
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

		{name: "test multiple if", yamlStr: `
exec:
- if: matched
  exec: [exec,exec,exec]
- if: matched
  exec: exec_target
`,
			wantTarget: true, wantErr: nil},

		{name: "test if else_exec", yamlStr: `
exec:
- if: not_matched
  exec: exec_err
  else_exec: exec_target
`,
			wantTarget: true, wantErr: nil},

		{name: "test multi if else_exec", yamlStr: `
exec:
- if: not_matched
  exec: [exec_err]
  else_exec: [exec]
- if: not_matched
  exec: [exec_err]
  else_exec: [exec_target]
`,
			wantTarget: true, wantErr: nil},

		{name: "test nested if", yamlStr: `
exec:
- if: matched
  exec: 
  - exec
  - exec
  - if: matched
    exec: exec_target
`,
			wantTarget: true, wantErr: nil},

		{name: "test if err", yamlStr: `
exec:
- if: "not_matched || match_err" # err
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
- if: matched 
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
- if: matched
  exec: 
    - exec_skip
    - exec_err # skipped, should not reach here.
- exec_err
`,
			wantTarget: false, wantErr: nil},
	}

	matchers := make(map[string]Matcher)
	execs := make(map[string]Executable)

	// not_matched
	matchers["not_matched"] = &DummyMatcher{
		Matched: false,
		WantErr: nil,
	}
	// matched
	matchers["matched"] = &DummyMatcher{
		Matched: true,
		WantErr: nil,
	}

	// matcher returns an error
	matchers["match_err"] = &DummyMatcher{
		Matched: false,
		WantErr: mErr,
	}

	// do something
	execs["exec"] = &DummyExecutable{
		WantErr: nil,
	}

	execs["exec_target"] = &DummyExecutable{
		WantR:   target,
		WantErr: nil,
	}

	// do something and skip the following sequence
	execs["exec_skip"] = &DummyExecutable{
		WantSkip: true,
		WantErr:  nil,
	}

	execs["exec_err"] = &DummyExecutable{
		WantErr: eErr,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := make(map[string]interface{}, 0)
			err := yaml.Unmarshal([]byte(tt.yamlStr), args)
			if err != nil {
				t.Fatal(err)
			}

			ecs, err := BuildExecutableLogicTree(args["exec"], zap.NewNop(), execs, matchers)
			if err != nil {
				t.Fatal(err)
			}

			qCtx := query_context.NewContext(new(dns.Msg), nil)
			err = ExecChainNode(context.Background(), qCtx, ecs)
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

	execs := make(map[string]Executable)
	execs["exec"] = &DummyExecutable{
		WantErr: nil,
	}

	execs["exec_target"] = &DummyExecutable{
		WantR:   target,
		WantErr: nil,
	}

	execs["exec_skip"] = &DummyExecutable{
		WantSkip: true,
		WantErr:  nil,
	}

	execs["exec_err"] = &DummyExecutable{
		WantErr: eErr,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := make(map[string]interface{}, 0)
			err := yaml.Unmarshal([]byte(tt.yamlStr), args)
			if err != nil {
				t.Fatal(err)
			}

			ecs, err := BuildExecutableLogicTree(args["exec"], zap.NewNop(), execs, nil)
			if err != nil {
				t.Fatal(err)
			}

			for r := 0; r < 3; r++ {
				for _, want := range tt.want {
					qCtx := query_context.NewContext(new(dns.Msg), nil)
					err = ExecChainNode(context.Background(), qCtx, ecs)
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
