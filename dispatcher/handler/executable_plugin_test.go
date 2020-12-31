package handler

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"testing"
)

func Test_switchPlugin_Do(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	mErr := errors.New("")
	eErr := errors.New("")

	notMatched := "not_matched"
	matched := "matched"
	matchErr := "match_err"

	exec := executablePluginTag("exec")
	execErr := executablePluginTag("exec_err")
	type args struct {
		executable ExecutableCmd
	}

	var tests = []struct {
		name     string
		args     *args
		wantNext string
		wantErr  error
	}{
		{name: "try to reach empty end", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifMatcher:     []string{"!" + matched, notMatched}, // not matched
					executableCmd: &ExecutableCmdSequence{execErr},
					goTwo:         "goto",
				},
			},
		}, wantNext: "", wantErr: nil},

		{name: "test if_and", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifAndMatcher: []string{matched, notMatched}, // not matched
					goTwo:        "goto1",
				},
				&ifBlock{
					ifAndMatcher: []string{matched, notMatched, matchErr}, // not matched, early stop, no err
					goTwo:        "goto2",
				},
				&ifBlock{
					ifAndMatcher: []string{matched, matched, matched}, // matched
					goTwo:        "goto3",
				},
			},
		}, wantNext: "goto3", wantErr: nil},

		{name: "test if_and err", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifAndMatcher: []string{matched, matchErr}, // err
					goTwo:        "goto1",
				},
			},
		}, wantNext: "", wantErr: mErr},

		{name: "test if", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifMatcher: []string{"!" + matched, notMatched}, // test ! prefix, not matched
					goTwo:     "goto1",
				},
				&ifBlock{
					ifMatcher: []string{matched, matchErr}, // matched, early stop, no err.
					executableCmd: &ExecutableCmdSequence{
						exec,
						&ifBlock{
							ifMatcher: []string{"!" + notMatched, matchErr}, // // test ! prefix, matched, early stop, no err.
							goTwo:     "goto2",                              // reached here
						},
					},
					goTwo: "goto3",
				},
			},
		}, wantNext: "goto2", wantErr: nil},

		{name: "test if matcher err", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifMatcher:     []string{"!" + matched, notMatched, matchErr}, // matcher err
					executableCmd: &ExecutableCmdSequence{exec},
					goTwo:         "goto",
				},
			},
		}, wantNext: "", wantErr: mErr},

		{name: "test exec err", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifMatcher:     []string{"!" + matched, matched},
					executableCmd: &ExecutableCmdSequence{execErr},
					goTwo:         "goto",
				},
			},
		}, wantNext: "", wantErr: eErr},
	}

	mustSuccess := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	// notMatched
	mustSuccess(RegPlugin(WrapMatcherPlugin(notMatched, "",
		&DummyMatcher{Matched: false, WantErr: nil},
	)))

	// do something
	mustSuccess(RegPlugin(WrapExecutablePlugin(string(exec), "",
		&DummyExecutable{WantErr: nil},
	)))

	// matched
	mustSuccess(RegPlugin(WrapMatcherPlugin(matched, "",
		&DummyMatcher{Matched: true, WantErr: nil},
	)))

	// plugins should return an err.
	mustSuccess(RegPlugin(WrapMatcherPlugin(matchErr, "",
		&DummyMatcher{Matched: false, WantErr: mErr},
	)))
	mustSuccess(RegPlugin(WrapExecutablePlugin(string(execErr), "",
		&DummyExecutable{WantErr: eErr},
	)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNext, err := tt.args.executable.ExecCmd(context.Background(), nil, mlog.NewPluginLogger("test"))
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
