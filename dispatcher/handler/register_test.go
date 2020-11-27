package handler

import (
	"context"
	"errors"
	"testing"
)

type dummyPlugin struct {
	t *testing.T

	next               string
	hasErr             bool
	shouldNoTBeReached bool
}

func (d *dummyPlugin) Tag() string {
	return "dummy plugin"
}

func (d *dummyPlugin) Type() string {
	return "dummy plugin"
}

func (d *dummyPlugin) Do(_ context.Context, _ *Context) (next string, err error) {
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
	tests := []struct {
		name     string
		entryTag string
		wantErr  bool
	}{
		{"normal exec sequence", "p1", false},
		{"endless loop exec sequence", "e1", true},
		{"err exec sequence", "err1", true},
	}

	// add a normal exec sequence
	pluginRegister["p1"] = &dummyPlugin{next: "p2"}
	pluginRegister["p2"] = &dummyPlugin{next: "p3"}
	pluginRegister["p3"] = &dummyPlugin{next: ""} // the end

	// add a endless loop exec sequence
	pluginRegister["e1"] = &dummyPlugin{next: "e2"}
	pluginRegister["e2"] = &dummyPlugin{next: "e3"}
	pluginRegister["e3"] = &dummyPlugin{next: "e1"} // endless loop

	// add a exec sequence which raise an err
	pluginRegister["err1"] = &dummyPlugin{next: "err2"}
	pluginRegister["err2"] = &dummyPlugin{next: "err3", hasErr: true}
	pluginRegister["err3"] = &dummyPlugin{next: "", shouldNoTBeReached: true}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Walk(ctx, nil, tt.entryTag); (err != nil) != tt.wantErr {
				t.Errorf("Walk() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
