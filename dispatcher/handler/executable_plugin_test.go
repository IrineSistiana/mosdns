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
					ifMather:      []string{"!" + matched, notMatched},
					executableCmd: &ExecutableCmdSequence{execErr},
					goTwo:         "goto",
				},
			},
		}, wantNext: "", wantErr: nil},

		{name: "try to reach goto 1", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifMather:      []string{"!" + matched, notMatched}, // not matched
					executableCmd: &ExecutableCmdSequence{execErr},
					goTwo:         "goto1",
				},
				&ifBlock{
					ifMather:      []string{matched, notMatched}, // matched
					executableCmd: nil,
					goTwo:         "",
				},
				&ifBlock{
					ifMather: []string{"!" + matched, matched, matchErr}, // matched, no err
					executableCmd: &ExecutableCmdSequence{
						exec,
						&ifBlock{
							ifMather:      []string{"!" + matched, notMatched, matched}, // matched
							executableCmd: &ExecutableCmdSequence{exec},
							goTwo:         "goto2", // reached here
						},
					},
					goTwo: "goto3",
				},
			},
		}, wantNext: "goto2", wantErr: nil},

		{name: "matcher err", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifMather:      []string{"!" + matched, notMatched, matchErr},
					executableCmd: &ExecutableCmdSequence{exec},
					goTwo:         "goto",
				},
			},
		}, wantNext: "", wantErr: mErr},
		{name: "exec err", args: &args{
			executable: &ExecutableCmdSequence{exec, exec,
				&ifBlock{
					ifMather:      []string{"!" + matched, matched},
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
