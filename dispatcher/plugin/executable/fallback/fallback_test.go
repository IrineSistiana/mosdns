package fallback

import (
	"testing"
)

func Test_fallback_updatePrimaryStat(t *testing.T) {
	tests := []struct {
		name   string
		in     []stat
		wantOK bool
	}{
		{"start0", nil, true},
		{"start1", []stat{success}, true},
		{"start2", []stat{success, success}, true},
		{"start3", []stat{success, success, success}, true},
		{"start4", []stat{failed, failed}, true},
		{"start5", []stat{failed, failed, failed}, false},
		{"start6", []stat{failed, failed, failed, success}, false},
		{"run1", []stat{failed, failed, failed, success, success}, true},
		{"run2", []stat{failed, failed, failed, failed, failed, failed, success, success}, true},
		{"run3", []stat{success, success, success, success, success, failed, failed, failed}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := newFallback("", &Args{
				Primary:    []interface{}{""},
				Secondary:  []interface{}{""},
				StatLength: 4,
				Threshold:  3,
			})
			if err != nil {
				t.Fatal(err)
			}

			for _, s := range tt.in {
				f.updatePrimaryStat(s)
			}

			if f.primaryIsOk() != tt.wantOK {
				t.Fatal()
			}
		})
	}
}
