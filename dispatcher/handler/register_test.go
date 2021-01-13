package handler

import (
	"errors"
	"reflect"
	"testing"
)

func TestRegPlugin(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	shutdownErr := errors.New("err")
	tests := []struct {
		name      string
		p         Plugin
		errOnDup  bool
		wantErr   bool
		wantPanic bool
	}{
		{"reg p1", &BP{tag: "p1"}, true, false, false},
		{"test err on dup p1", &BP{tag: "p1"}, true, true, false},
		{"reg service plugin sp1", &DummyServicePlugin{BP: NewBP("sp1", "")}, true, false, false},
		{"reg err service plugin sp2", &DummyServicePlugin{BP: NewBP("sp2", ""), WantShutdownErr: shutdownErr}, true, false, false},
		{"test shutdown service sp1", &BP{tag: "sp1"}, false, false, false},
		{"test shutdown service sp2", &BP{tag: "sp2"}, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			func() {
				if tt.wantPanic {
					defer func() {
						err := recover()
						if err == nil {
							t.Error("test should panic")
						}
					}()
				}

				err := RegPlugin(tt.p, tt.errOnDup)
				if (err != nil) != tt.wantErr {
					t.Errorf("RegPlugin() error = %v, wantErr %v", err, tt.wantErr)
				}
				if err != nil {
					return
				}

				gotP, err := GetPlugin(tt.p.Tag())
				if err != nil {
					t.Errorf("failed to get registed plugin")
				}
				if !reflect.DeepEqual(gotP.GetPlugin(), tt.p) {
					t.Errorf("want p %v, but got %v", tt.p, gotP)
				}
			}()
		})
	}
}

func Test_pluginRegister_delPlugin(t *testing.T) {
	tests := []struct {
		name      string
		p         Plugin
		tag       string
		wantPanic bool
	}{
		{"del matcher", &DummyMatcherPlugin{
			BP: NewBP("test", ""),
		}, "test", false},
		{"del service", &DummyServicePlugin{
			BP: NewBP("test", ""),
		}, "test", false},
		{"del service but panic", &DummyServicePlugin{
			BP:              NewBP("test", ""),
			WantShutdownErr: errors.New(""),
		}, "test", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newPluginRegister()
			err := r.regPlugin(tt.p, true)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantPanic {
				defer func() {
					msg := recover()
					if msg == nil {
						t.Error("delPlugin not panic")
					}
				}()
			}

			r.delPlugin(tt.tag)
			if _, err := GetPlugin(tt.tag); err == nil {
				t.Error("plugin is not deleted")
			}
		})

	}
}
