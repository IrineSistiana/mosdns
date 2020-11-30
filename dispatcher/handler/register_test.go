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
	pluginTagRegister.rpRegister["p1"] = &dummySequencePlugin{next: "p2"}
	pluginTagRegister.rpRegister["p2"] = &dummySequencePlugin{next: "p3"}
	pluginTagRegister.rpRegister["p3"] = &dummySequencePlugin{next: ""} // the end

	pluginTagRegister.rpRegister["end1"] = &dummySequencePlugin{next: StopSignTag} // the end

	// add a endless loop exec sequence
	pluginTagRegister.rpRegister["e1"] = &dummySequencePlugin{next: "e2"}
	pluginTagRegister.rpRegister["e2"] = &dummySequencePlugin{next: "e3"}
	pluginTagRegister.rpRegister["e3"] = &dummySequencePlugin{next: "e1"} // endless loop

	// add a exec sequence which raise an err
	pluginTagRegister.rpRegister["err1"] = &dummySequencePlugin{next: "err2"}
	pluginTagRegister.rpRegister["err2"] = &dummySequencePlugin{next: "err3", hasErr: true}
	pluginTagRegister.rpRegister["err3"] = &dummySequencePlugin{next: "", shouldNoTBeReached: true}

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

func Test_entryRegister(t *testing.T) {
	r := &entryRegister{}

	eIn := []string{"a", "b", "c"}
	r.reg(eIn...)

	if eOut := r.get(); !reflect.DeepEqual(eIn, eOut) {
		t.Fatalf("r.get() test, want %v, got %v", eIn, eOut)
	}
}

func Test_entryRegister_reg(t *testing.T) {
	tests := []struct {
		name string
		in   []string
	}{
		{"empty entry", nil},
		{"one entry", []string{"a"}},
		{"many entries", []string{"a", "b", "c", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &entryRegister{}

			r.reg(tt.in...)
			if eOut := r.get(); !reflect.DeepEqual(tt.in, eOut) {
				t.Errorf("test failed, want %v, got %v", tt.in, eOut)
			}
		})
	}
}

func Test_entryRegister_get(t *testing.T) {
	r := &entryRegister{}
	eIn := []string{"a", "b", "c", "d", "abcdefg"}
	r.reg(eIn...)

	eOut := r.get()
	r.reg("x")                         // add one more entry
	if !reflect.DeepEqual(eIn, eOut) { // eOut should not be changed
		t.Errorf("test failed, want %v, got %v", eIn, eOut)
	}

	deleted, err := r.del("^a(?:\\w)+g$")
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "abcdefg" {
		t.Fatal("entry is not deleted")
	}

	if !reflect.DeepEqual(eIn, eOut) { // eOut should not be changed
		t.Errorf("test failed, want %v, got %v", eIn, eOut)
	}
}

func Test_entryRegister_del(t *testing.T) {
	type fields struct {
		e []string
	}
	type args struct {
		entryRegexp string
	}

	empty := []string{}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantDeleted []string
		wantGet     []string
		wantErr     bool
	}{
		{"empty r", fields{e: nil}, args{entryRegexp: "\\w*"}, nil, empty, false},
		{"empty r 2", fields{e: empty}, args{entryRegexp: "\\w*"}, nil, empty, false},
		{"invalid expr", fields{e: empty}, args{entryRegexp: "*"}, nil, empty, true},
		{"del none r", fields{e: []string{"a", "b"}}, args{entryRegexp: "c"}, nil, []string{"a", "b"}, false},
		{"del one", fields{e: []string{"a", "b"}}, args{entryRegexp: "a"}, []string{"a"}, []string{"b"}, false},
		{"del many", fields{e: []string{"a", "aa", "aaa", "b"}}, args{entryRegexp: "^a"}, []string{"a", "aa", "aaa"}, []string{"b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &entryRegister{
				e: tt.fields.e,
			}
			gotDeleted, err := r.del(tt.args.entryRegexp)
			if (err != nil) != tt.wantErr {
				t.Errorf("del() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(gotDeleted, tt.wantDeleted) {
				t.Errorf("del() gotDeleted = %v, want %v", gotDeleted, tt.wantDeleted)
			}

			gotGet := r.get()
			if !reflect.DeepEqual(gotGet, tt.wantGet) {
				t.Errorf("get() gotGet = %v, want %v", gotGet, tt.wantGet)
			}
		})
	}
}

func BenchmarkHandler(b *testing.B) {
	PurgePluginRegister()
	defer PurgePluginRegister()
	PurgeEntry()
	defer PurgeEntry()

	RegEntry("0")

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
			err := Dispatch(context.Background(), nil)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
