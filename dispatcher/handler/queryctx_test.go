package handler

import (
	"context"
	"errors"
	"github.com/miekg/dns"
	"testing"
)

func TestContext_defer(t *testing.T) {
	exec := &DummyExecutablePlugin{
		BP: NewBP("test", ""),
	}
	errExec := &DummyExecutablePlugin{
		BP:      NewBP("test", ""),
		WantErr: errors.New(""),
	}

	tests := []struct {
		name       string
		exec       []Executable
		wantRemain int
		wantErr    bool
	}{
		{"no defer", []Executable{}, 0, false},
		{"1 defer", []Executable{exec}, 0, false},
		{"3 defer", []Executable{exec, exec, exec}, 0, false},
		{"defer err", []Executable{exec, errExec}, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(new(dns.Msg), nil)
			for _, e := range tt.exec {
				ctx.DeferExec(e)
			}
			err := ctx.ExecDefer(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecDefer() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(ctx.deferrable) != tt.wantRemain {
				t.Errorf("ExecDefer() remain = %v, wantErr %v", len(ctx.deferrable), tt.wantRemain)
			}
		})
	}
}

type invalidExecutable struct {
	execDefer bool
}

func (i invalidExecutable) Exec(ctx context.Context, qCtx *Context) (err error) {
	if i.execDefer {
		return qCtx.ExecDefer(ctx)
	}
	qCtx.DeferExec(i)
	return nil
}

func TestContext_defer_panic(t *testing.T) {
	tests := []struct {
		name      string
		exec      Executable
		wantPanic bool
	}{
		{"ExecDefer", invalidExecutable{true}, true},
		{"DeferExec", invalidExecutable{false}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(new(dns.Msg), nil)
			ctx.DeferExec(tt.exec)

			defer func() {
				msg := recover()
				if msg == nil {
					t.Error("not panic")
				}
			}()
			ctx.ExecDefer(context.Background())
		})
	}
}
