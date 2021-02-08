package elem

import "testing"

func TestIntMatcher_Match(t *testing.T) {
	tests := []struct {
		name string
		m    []int
		args int
		want bool
	}{
		{"nil 1", nil, 1, false},
		{"matched 1", []int{1, 2, 3}, 1, true},
		{"matched 2", []int{1, 2, 3}, 3, true},
		{"not matched 1", []int{1, 2, 3}, 0, false},
		{"not matched 2", []int{1, 2, 3}, 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewIntMatcher(tt.m)
			if got := m.Match(tt.args); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}
