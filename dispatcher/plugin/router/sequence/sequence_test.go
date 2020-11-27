package sequence

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"testing"
)

func Test_switchPlugin_Do(t *testing.T) {

	mErr := errors.New("")
	eErr := errors.New("")

	notMatched := "not_matched"
	matched := "matched"
	matchErr := "match_err"

	exec := "exec"
	execErr := "exec_err"

	var tests = []struct {
		name     string
		args     *Args
		wantNext string
		wantErr  error
	}{
		{name: "try to reach next 1", args: &Args{
			Sequence: []*Block{
				{
					If:   matched,
					Exec: exec,
					Sequence: []*Block{{
						If:       matched,
						Exec:     exec,
						Sequence: nil,
						Goto:     "",
					}},
					Goto: "",
				},
			},
			Next: "no_rd",
		}, wantNext: "no_rd", wantErr: nil},
		{name: "try to reach goto 1", args: &Args{
			Sequence: []*Block{
				{
					If:   matched,
					Exec: exec,
					Sequence: []*Block{{
						If:       "",
						Exec:     exec,
						Sequence: nil,
						Goto:     "goto",
					}},
					Goto: "",
				},
			},
			Next: "no_rd",
		}, wantNext: "goto", wantErr: nil},
		{name: "try to reach goto 2", args: &Args{
			Sequence: []*Block{
				{
					If:   matched,
					Exec: exec,
					Sequence: []*Block{{
						If:       notMatched, // not matched
						Exec:     execErr,    // should not reach this exec
						Sequence: nil,
						Goto:     "goto1", // should not reach this end
					}, {
						If:       matched,
						Exec:     exec,
						Sequence: nil,
						Goto:     "goto2",
					}},
					Goto: "",
				},
			},
			Next: "no_rd",
		}, wantNext: "goto2", wantErr: nil},
		{name: "try to reach goto 3", args: &Args{
			Sequence: []*Block{
				{
					If:   "!" + notMatched, // matched
					Exec: exec,
					Sequence: []*Block{{
						If:       "!" + matched, // not matched
						Exec:     execErr,       // should not reach this exec
						Sequence: nil,
						Goto:     "goto1", // should not reach this end
					}, {
						If:       matched,
						Exec:     exec,
						Sequence: nil,
						Goto:     "goto2",
					}},
					Goto: "",
				},
			},
			Next: "no_rd",
		}, wantNext: "goto2", wantErr: nil},
		{name: "matcher err", args: &Args{
			Sequence: []*Block{
				{
					If:   matched,
					Exec: exec,
					Sequence: []*Block{{
						If:       matchErr,
						Exec:     execErr,
						Sequence: nil,
						Goto:     "goto1",
					}, {
						If:       matched,
						Exec:     exec,
						Sequence: nil,
						Goto:     "goto2",
					}},
					Goto: "",
				},
			},
			Next: "no_rd",
		}, wantNext: "", wantErr: mErr},
		{name: "exec err", args: &Args{
			Sequence: []*Block{
				{
					If:   matched,
					Exec: exec,
					Sequence: []*Block{{
						If:       matched,
						Exec:     execErr,
						Sequence: nil,
						Goto:     "goto1",
					}, {
						If:       matched,
						Exec:     exec,
						Sequence: nil,
						Goto:     "goto2",
					}},
					Goto: "",
				},
			},
			Next: "no_rd",
		}, wantNext: "", wantErr: eErr},
	}

	mustSuccess := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	// notMatched
	mustSuccess(handler.RegPlugin(handler.WrapMatcherPlugin(notMatched, "",
		&handler.DummyMatcher{Matched: false, WantErr: nil},
	)))

	// do something
	mustSuccess(handler.RegPlugin(handler.WrapFunctionalPlugin(exec, "",
		&handler.DummyFunctional{WantErr: nil},
	)))

	// matched
	mustSuccess(handler.RegPlugin(handler.WrapMatcherPlugin(matched, "",
		&handler.DummyMatcher{Matched: true, WantErr: nil},
	)))

	// plugins should return an err.
	mustSuccess(handler.RegPlugin(handler.WrapMatcherPlugin(matchErr, "",
		&handler.DummyMatcher{Matched: false, WantErr: mErr},
	)))
	mustSuccess(handler.RegPlugin(handler.WrapFunctionalPlugin(execErr, "",
		&handler.DummyFunctional{WantErr: eErr},
	)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sequence{
				args: tt.args,
			}
			gotNext, err := s.Do(context.Background(), nil)

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
