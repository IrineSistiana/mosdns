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

func TestWalk(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	tests := []struct {
		name     string
		entryTag string
		wantErr  bool
	}{
		{"normal exec sequence 1", "p1", false},
		{"normal exec sequence 2", "end1", false},
		{"endless loop exec sequence", "e1", true},
		{"err exec sequence", "err1", true},
	}

	// add a normal exec sequence
	routerPluginRegister["p1"] = &dummySequencePlugin{next: "p2"}
	routerPluginRegister["p2"] = &dummySequencePlugin{next: "p3"}
	routerPluginRegister["p3"] = &dummySequencePlugin{next: ""} // the end

	routerPluginRegister["end1"] = &dummySequencePlugin{next: StopSignTag} // the end

	// add a endless loop exec sequence
	routerPluginRegister["e1"] = &dummySequencePlugin{next: "e2"}
	routerPluginRegister["e2"] = &dummySequencePlugin{next: "e3"}
	routerPluginRegister["e3"] = &dummySequencePlugin{next: "e1"} // endless loop

	// add a exec sequence which raise an err
	routerPluginRegister["err1"] = &dummySequencePlugin{next: "err2"}
	routerPluginRegister["err2"] = &dummySequencePlugin{next: "err3", hasErr: true}
	routerPluginRegister["err3"] = &dummySequencePlugin{next: "", shouldNoTBeReached: true}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Walk(ctx, nil, tt.entryTag); (err != nil) != tt.wantErr {
				t.Errorf("Walk() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
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

	fp := WrapFunctionalPlugin("fp", "", &DummyFunctional{})
	mp := WrapMatcherPlugin("wp", "", &DummyMatcher{})
	sp := &dummySequencePlugin{}
	jp := &justPlugin{}

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
		{"reg jp", args{p: jp}, nil, true},
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

			p, ok := GetPlugin(tt.args.p.Tag())
			if !ok {
				t.Errorf("failed to get registed plugin")
			}
			if !reflect.DeepEqual(p, tt.wantPlugin) {
				t.Errorf("want p %v, but got %v", tt.wantPlugin, p)
			}
		})
	}
}
