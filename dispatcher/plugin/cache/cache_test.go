package cache

import (
	"github.com/miekg/dns"
	"strconv"
	"testing"
	"time"
)

func Test_cache(t *testing.T) {
	c := newCache(8, time.Millisecond*50)
	for i := 0; i < 8; i++ {
		m := new(dns.Msg)
		m.SetQuestion(strconv.Itoa(i)+".", dns.TypeA)
		c.add(strconv.Itoa(i), 1, m)
	}

	for i := 0; i < 8; i++ {
		r, _ := c.get(strconv.Itoa(i))
		if r.Question[0].Name != strconv.Itoa(i)+"." {
			t.Fatal()
		}
	}

	if c.len() != 8 {
		t.Fatal()
	}

	c.add(strconv.Itoa(9), 1, nil)
	if c.len() != 8 {
		t.Fatal()
	}

	time.Sleep(time.Millisecond * 1200)
	if c.len() != 0 {
		t.Fatal()
	}

	if c.cleanerIsRunning != false {
		t.Fatal()
	}

}
