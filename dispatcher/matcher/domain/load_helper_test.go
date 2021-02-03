package domain

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func Test_containAttr(t *testing.T) {
	type args struct {
		attr []string
		want []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"1", args{attr: []string{"a"}, want: []string{"a"}}, true},
		{"2", args{attr: []string{"a", "b"}, want: []string{"a"}}, true},
		{"3", args{attr: []string{"a", "b"}, want: []string{"a", "b"}}, true},
		{"4", args{attr: []string{"a", "b", "c"}, want: []string{"b", "c"}}, true},
		{"5", args{attr: []string{"a", "b", "c"}, want: []string{"a", "d"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustHaveAttr(tt.args.attr, tt.args.want); got != tt.want {
				t.Errorf("mustHaveAttr() = %v, want %v", got, tt.want)
			}
		})
	}
}

type dummyMatcher struct {
	wantPattern string
	wantV       interface{}
}

func (d *dummyMatcher) Match(fqdn string) (v interface{}, ok bool) {
	panic("not implement")
}

func (d *dummyMatcher) Len() int {
	panic("not implement")
}

func (d *dummyMatcher) Add(patten string, v interface{}) error {
	if patten != d.wantPattern {
		return fmt.Errorf("matcher Add(): want pattern %s, got %s", d.wantPattern, patten)
	}

	if !reflect.DeepEqual(d.wantV, v) {
		return fmt.Errorf("matcher Add(): want V %v, got %v", d.wantV, v)
	}
	return nil
}

func (d *dummyMatcher) Del(patten string) {
	panic("not implement")
}

func TestLoadFromText(t *testing.T) {
	type args struct {
		m           Matcher
		s           string
		processAttr ProcessAttrFunc
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "space",
			args: args{
				s:           "   ",
				processAttr: nil,
				m: &dummyMatcher{
					wantPattern: "",
					wantV:       struct{}{}, // matcher should not be called
				},
			},
		},
		{
			name: "space and comment",
			args: args{
				s:           "   #string",
				processAttr: nil,
				m: &dummyMatcher{
					wantPattern: "",
					wantV:       struct{}{}, // matcher should not be called
				},
			},
		},
		{
			name: "space and data",
			args: args{
				s:           "   data attr1 attr2 # comment",
				processAttr: nil,
				m: &dummyMatcher{
					wantPattern: "data",
					wantV:       nil,
				},
			},
		},
		{
			name: "processAttr accepted",
			args: args{
				s: "   data    attr1    attr2 # comment",
				processAttr: func(strings []string) (v interface{}, accept bool, err error) {
					return strings, true, nil
				},
				m: &dummyMatcher{
					wantPattern: "data",
					wantV:       []string{"attr1", "attr2"},
				},
			},
		},
		{
			name: "processAttr denied",
			args: args{
				s: "   data    attr1    attr2 # comment",
				processAttr: func(strings []string) (v interface{}, accept bool, err error) {
					return nil, false, nil
				},
				m: &dummyMatcher{
					wantPattern: "data",
					wantV:       struct{}{}, // should not be called
				},
			},
		},

		{
			name: "processAttr err",
			args: args{
				s: "   data    attr1    attr2 # comment",
				processAttr: func(strings []string) (v interface{}, accept bool, err error) {
					return nil, false, errors.New("")
				},
				m: &dummyMatcher{
					wantPattern: "",
					wantV:       struct{}{}, // should not be called
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := LoadFromText(tt.args.m, tt.args.s, tt.args.processAttr); (err != nil) != tt.wantErr {
				t.Errorf("LoadFromText() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
