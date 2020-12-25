package domain

import (
	"testing"
	"v2ray.com/core/app/router"
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
		var attr []*router.Domain_Attribute
		for _, key := range tt.args.attr {
			attr = append(attr, &router.Domain_Attribute{Key: key})
		}

		t.Run(tt.name, func(t *testing.T) {
			if got := containAttr(attr, tt.args.want); got != tt.want {
				t.Errorf("containAttr() = %v, want %v", got, tt.want)
			}
		})
	}
}
