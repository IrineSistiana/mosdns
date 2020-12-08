package handler

import (
	"context"
	"errors"
	"reflect"
	"strconv"
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

func BenchmarkHandler(b *testing.B) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	for i := 0; i < 40; i++ {
		p := &DummyRouterPlugin{
			TagStr:   strconv.Itoa(i),
			WantNext: strconv.Itoa(i + 1),
			WantErr:  nil,
		}
		err := RegPlugin(p)
		if err != nil {
			b.Fatal(err)
		}
	}
	err := RegPlugin(&DummyRouterPlugin{
		TagStr:   strconv.Itoa(40),
		WantNext: "",
		WantErr:  nil,
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := Walk(context.Background(), nil, "0")
			if err != nil {
				panic(err.Error())
			}
		}
	})

	r := pluginTagRegister
	r.Lock()
	r.Unlock()
}
