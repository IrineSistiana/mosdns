package coremain

import (
	"fmt"
	"testing"
)

func Test_runAndReplace(t *testing.T) {
	echo := func(s string) ([]byte, error) {
		return []byte(s), nil
	}

	deadLoopEcho := func(s string) ([]byte, error) {
		return []byte(fmt.Sprintf("${{%s}}", s)), nil
	}

	tests := []struct {
		name       string
		in         string
		handleFunc func(s string) ([]byte, error)
		want       string
		wantErr    bool
	}{
		{"", "${{123}}", echo, "123", false},
		{"", "${{      123  }}", echo, "123", false},
		{"", "${{ 123 }}", echo, "123", false},
		{"", "123${{ 456 }}789", echo, "123456789", false},
		{"", "${{ 123 }} ${{ 456 }}", echo, "123 456", false},
		{"", "${{ 123 ${{ 456 }} }}", echo, "123 456", false},
		{"", "${{123}}\n456", echo, "123\n456", false},
		{"comment", "123#456", echo, "123#456", false},
		{"comment", "123 #456", echo, "123 #456", false},
		{"comment", "#456}}${{ 123 }}", echo, "#456}}${{ 123 }}", false},
		{"comment", "${{ 123 }} #456}}", echo, "123 #456}}", false},
		{"nest", "${{ 123 ${{ 456 }} }}", echo, "123 456", false},
		{"nest", "${{ 123${{456${{789}}}}}}", echo, "123456789", false},
		{"dead loop", "${{ 123 }}", deadLoopEcho, "", true},
		{"invalid syntax", "${{ 123 }}}}", echo, "", true},
		{"invalid syntax", "${{ 123 }", echo, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := findAndReplaceCmd([]byte(tt.in), 0, tt.handleFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("findAndReplaceCmd() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := string(b)
			if got != tt.want {
				t.Errorf("findAndReplaceCmd() got = %v, want %v", got, tt.want)
			}
		})
	}
}
