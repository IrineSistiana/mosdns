package domain

import (
	"testing"
)

func TestDomainMatcher(t *testing.T) {
	m := NewDomainMatcher()
	add := func(domain string, v interface{}) {
		m.Add(domain, v)
	}
	assert := assertFunc(t, m)

	add("cn", nil)
	assert("cn", true, nil)
	assert("a.cn", true, nil)
	assert("a.com", false, nil)
	add("a.b.com", nil)
	assert("a.b.com", true, nil)
	assert("q.w.e.r.a.b.com", true, nil)
	assert("b.com", false, nil)

	// test replace
	add("append", 0)
	assert("append", true, 0)
	add("append", 1)
	assert("append", true, 1)
	add("append", nil)
	assert("append", true, nil)

	// test appendable
	add("append", nil)
	assert("a.append", true, nil)
	add("append", s("a"))
	assert("b.append", true, s("a"))
	add("append", s("b"))
	assert("c.append", true, s("ab"))
	add("c.append", s("c")) // redundant
	assert("c.append", true, s("ab"))

	assertInt(t, m.Len(), 3)
}

func TestDomainMatcherRoot(t *testing.T) {
	m := NewDomainMatcher()
	if err := m.Add(".", nil); err != nil {
		t.Fatal(err)
	}
	for _, fqdn := range []string{"com.", "test.com."} {
		_, ok := m.Match(fqdn)
		if !ok {
			t.Fatal()
		}
	}
}

func assertInt(t testing.TB, want, got int) {
	if want != got {
		t.Errorf("assertion failed: want %d, got %d", want, got)
	}
}

func TestPrevLabel(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		wantLabel  string
		wantRemain string
	}{
		{"", "test.com", "com", "test"},
		{"", "com", "com", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := SplitLatestLabel(tt.s)
			if got != tt.wantLabel {
				t.Errorf("SplitLatestLabel() got = %v, want %v", got, tt.wantLabel)
			}
			if got1 != tt.wantRemain {
				t.Errorf("SplitLatestLabel() got1 = %v, want %v", got1, tt.wantRemain)
			}
		})
	}
}
