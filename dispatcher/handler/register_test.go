package handler

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type dummySequencePlugin struct {
	t *testing.T

	next               string
	hasErr             bool
	shouldNoTBeReached bool
}

func (d *dummySequencePlugin) Tag() string {
	return "dummy plugin"
}

func (d *dummySequencePlugin) Type() string {
	return "dummy plugin"
}

func (d *dummySequencePlugin) Do(_ context.Context, _ *Context) (next string, err error) {
	if d.shouldNoTBeReached {
		d.t.Fatal("exec sequence reached unreachable plugin")
	}

	next = d.next
	if d.hasErr {
		err = errors.New("err")
	}
	return
}

func mustSuccess(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

type justPlugin struct{}

func (j *justPlugin) Tag() string {
	return ""
}

func (j *justPlugin) Type() string {
	return ""
}

func TestRegPlugin(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	fp := WrapExecutablePlugin("fp", "", &DummyExecutable{})
	mp := WrapMatcherPlugin("wp", "", &DummyMatcher{})
	sp := &dummySequencePlugin{}

	type args struct {
		p Plugin
	}
	tests := []struct {
		name       string
		args       args
		wantPlugin Plugin
		wantErr    bool
	}{
		{"reg fp", args{p: fp}, fp, false},
		{"reg mp", args{p: mp}, mp, false},
		{"reg sp", args{p: sp}, sp, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegPlugin(tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegPlugin() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			p, err := GetPlugin(tt.args.p.Tag())
			if err != nil {
				t.Errorf("failed to get registed plugin")
			}
			if !reflect.DeepEqual(p, tt.wantPlugin) {
				t.Errorf("want p %v, but got %v", tt.wantPlugin, p)
			}
		})
	}
}
