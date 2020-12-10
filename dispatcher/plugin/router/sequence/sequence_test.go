package sequence

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"testing"
)

func Test_switchPlugin_Do(t *testing.T) {
	handler.PurgePluginRegister()
	defer handler.PurgePluginRegister()

	mErr := errors.New("")
	eErr := errors.New("")

	notMatched := "not_matched"
	matched := "matched"
	matchErr := "match_err"

	exec := functionalPlugin("exec")
	execErr := functionalPlugin("exec_err")
	type args struct {
		executable []executable
		Next       string
	}

	var tests = []struct {
		name     string
		args     *args
		wantNext string
		wantErr  error
	}{
		{name: "try to reach next 1", args: &args{
			executable: []executable{exec, exec,
				&ifBlock{
					ifMather:   []string{"!" + matched, notMatched},
					executable: []executable{execErr},
					gotoRouter: "goto",
				},
			},
			Next: "no_rd",
		}, wantNext: "no_rd", wantErr: nil},

		{name: "try to reach goto 1", args: &args{
			executable: []executable{exec, exec,
				&ifBlock{
					ifMather:   []string{"!" + matched, notMatched}, // not matched
					executable: []executable{execErr},
					gotoRouter: "goto1",
				},
				&ifBlock{
					ifMather: []string{"!" + matched, matched, matchErr}, // matched, no err
					executable: []executable{
						exec,
						&ifBlock{
							ifMather:   []string{"!" + matched, notMatched, matched}, // matched
							executable: []executable{exec},
							gotoRouter: "goto2", // reached here
						},
					},
					gotoRouter: "goto3",
				},
			},
			Next: "no_rd",
		}, wantNext: "goto2", wantErr: nil},

		{name: "matcher err", args: &args{
			executable: []executable{exec, exec,
				&ifBlock{
					ifMather:   []string{"!" + matched, notMatched, matchErr},
					executable: []executable{exec},
					gotoRouter: "goto",
				},
			},
			Next: "no_rd",
		}, wantNext: "", wantErr: mErr},
		{name: "exec err", args: &args{
			executable: []executable{exec, exec,
				&ifBlock{
					ifMather:   []string{"!" + matched, matched},
					executable: []executable{execErr},
					gotoRouter: "goto",
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
	mustSuccess(handler.RegPlugin(handler.WrapFunctionalPlugin(string(exec), "",
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
	mustSuccess(handler.RegPlugin(handler.WrapFunctionalPlugin(string(execErr), "",
		&handler.DummyFunctional{WantErr: eErr},
	)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			s := newSequencePlugin("", tt.args.executable, tt.args.Next)
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

func Test_yaml(t *testing.T) {

}
