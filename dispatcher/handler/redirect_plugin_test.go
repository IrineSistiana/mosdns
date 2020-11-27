package handler

import (
	"context"
	"testing"
)

type dummyChecker struct {
	res bool
}

func (d *dummyChecker) Match(_ context.Context, _ *Context) (matched bool, err error) {
	matched = d.res
	return
}

func Test_redirectPlugin_Do(t *testing.T) {
	next := "next"
	redirect := "redirect"

	tests := []struct {
		name     string
		checker  Checker
		wantNext string
		wantErr  bool
	}{
		{"matched", &dummyChecker{res: true}, redirect, false},
		{"not matched", &dummyChecker{res: false}, next, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &redirectPlugin{
				checker:  tt.checker,
				next:     next,
				redirect: redirect,
			}
			gotNext, err := c.Do(nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Do() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotNext != tt.wantNext {
				t.Errorf("Do() gotNext = %v, want %v", gotNext, tt.wantNext)
			}
		})
	}
}
