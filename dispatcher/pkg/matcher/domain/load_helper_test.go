package domain

import (
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
