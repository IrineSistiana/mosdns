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
