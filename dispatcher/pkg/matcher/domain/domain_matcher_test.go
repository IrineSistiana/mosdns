package domain

import (
	"reflect"
	"testing"
)

func TestDomainMatcher(t *testing.T) {
	m := NewDomainMatcher()
	add := func(domain string, v interface{}) {
		m.Add(domain, v)
	}
	assert := assertFunc(t, m)

	add("cn", nil)
	assertInt(t, 1, m.Len())
	assert("cn", true, nil)
	assert("a.cn.", true, nil)
	assert("a.com", false, nil)
	add("a.b.com", nil)
	assertInt(t, 2, m.Len())
	assert("a.b.com.", true, nil)
	assert("q.w.e.r.a.b.com.", true, nil)
	assert("b.com.", false, nil)

	// test replace
	add("append", 0)
	assertInt(t, 3, m.Len())
	assert("append.", true, 0)
	add("append.", 1)
	assert("append.", true, 1)
	add("append", nil)
	assert("append.", true, nil)

	// test appendable
	add("append", nil)
	assertInt(t, 3, m.Len())
	assert("a.append", true, nil)
	add("append", s("a"))
	assertInt(t, 3, m.Len())
	assert("b.append", true, s("a"))
	add("append", s("b"))
	assert("c.append", true, s("ab"))

	// test redundant data
	add("c.append", s("c")) // redundant
	assertInt(t, 3, m.Len())
	assert("c.append", true, s("ab"))
	add("a.a", 1)
	assertInt(t, 4, m.Len())
	add("a", 2) // parent redundant
	assertInt(t, 4, m.Len())
	assert("a.a", true, 2)
}

func assertInt(t testing.TB, want, got int) {
	if want != got {
		t.Errorf("assertion failed: want %d, got %d", want, got)
	}
}

func TestDomainScanner(t *testing.T) {
	tests := []struct {
		name           string
		fqdn           string
		wantOffsets    []int
		wantLabels     []string
		wantSubDomains []string
	}{
		{"empty", "", []int{}, []string{}, []string{}},
		{"root", ".", []int{}, []string{}, []string{}},
		{"non fqdn", "a.2", []int{2, 0}, []string{"2", "a"}, []string{"2", "a.2"}},
		{"domain", "1.2.3.", []int{4, 2, 0}, []string{"3", "2", "1"}, []string{"3", "2.3", "1.2.3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewUnifiedDomainScanner(tt.fqdn)
			gotOffsets := make([]int, 0)
			for s.Scan() {
				gotOffsets = append(gotOffsets, s.PrevLabelOffset())
			}
			if !reflect.DeepEqual(gotOffsets, tt.wantOffsets) {
				t.Errorf("PrevLabelOffset() = %v, want %v", gotOffsets, tt.wantOffsets)
			}

			s = NewUnifiedDomainScanner(tt.fqdn)
			gotLabels := make([]string, 0)
			for s.Scan() {
				pl, _ := s.PrevLabel()
				gotLabels = append(gotLabels, pl)
			}
			if !reflect.DeepEqual(gotLabels, tt.wantLabels) {
				t.Errorf("PrevLabel() = %v, want %v", gotLabels, tt.wantLabels)
			}

			s = NewUnifiedDomainScanner(tt.fqdn)
			gotSubDomains := make([]string, 0)
			for s.Scan() {
				sd, _ := s.PrevSubDomain()
				gotSubDomains = append(gotSubDomains, sd)
			}
			if !reflect.DeepEqual(gotSubDomains, tt.wantSubDomains) {
				t.Errorf("PrevLabel() = %v, want %v", gotSubDomains, tt.wantSubDomains)
			}
		})
	}
}

func TestFastDomainMatcher(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		want     []string
		dontWant []string
	}{
		{},
		{"b", "b", []string{"b", "a.b", "0.a.b"}, []string{"c", "cb"}},
		{"fqdn pattern", "b.", []string{"b", "a.b", "0.a.b"}, []string{"c", "cb"}},
		{"fqdn match", "b", []string{"b.", "a.b.", "0.a.b."}, []string{"c.", "cb."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewSimpleDomainMatcher()
			if err := m.Add(tt.domain, nil); err != nil {
				t.Fatal(err)
			}
			for _, s := range tt.want {
				if _, ok := m.Match(s); !ok {
					t.Fatalf("%s should match %s, but it did't", tt.domain, s)
				}
			}

			for _, s := range tt.dontWant {
				if _, ok := m.Match(s); ok {
					t.Fatalf("%s should not match %s, but it did", tt.domain, s)
				}
			}
		})
	}
}
