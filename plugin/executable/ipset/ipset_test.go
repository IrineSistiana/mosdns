//go:build linux

/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package ipset

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"github.com/vishvananda/netlink"
)

func skipTest(t *testing.T) {
	if os.Getenv("TEST_IPSET") == "" {
		t.SkipNow()
	}
}

func prepareSet(t *testing.T) (func(), string, string) {
	t.Helper()
	n4 := "test" + strconv.Itoa(rand.Int())
	n6 := "test" + strconv.Itoa(rand.Int())
	if err := netlink.IpsetCreate(n4, "hash:net", netlink.IpsetCreateOptions{
		Family: netlink.FAMILY_V4,
	}); err != nil {
		t.Fatal(err)
	}
	if err := netlink.IpsetCreate(n6, "hash:net", netlink.IpsetCreateOptions{
		Family: netlink.FAMILY_V6,
	}); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := netlink.IpsetDestroy(n4); err != nil {
			t.Fatal(err)
		}
		if err := netlink.IpsetDestroy(n6); err != nil {
			t.Fatal(err)
		}
	}, n4, n6
}

func Test_ipset(t *testing.T) {
	skipTest(t)

	done, n4, n6 := prepareSet(t)
	defer done()

	v, err := QuickSetup(nil, fmt.Sprintf("%s,inet,24 %s,inet6,48", n4, n6))
	if err != nil {
		t.Fatal(err)
	}
	p := v.(sequence.Executable)

	q := new(dns.Msg)
	q.SetQuestion("test.", dns.TypeA)
	r := new(dns.Msg)
	r.SetReply(q)
	r.Answer = append(r.Answer, &dns.A{A: net.ParseIP("127.0.0.1")})
	r.Answer = append(r.Answer, &dns.A{A: net.ParseIP("127.0.0.2")})
	r.Answer = append(r.Answer, &dns.AAAA{AAAA: net.ParseIP("::1")})
	r.Answer = append(r.Answer, &dns.AAAA{AAAA: net.ParseIP("::2")})
	qCtx := query_context.NewContext(q)
	qCtx.SetResponse(r)
	if err := p.Exec(context.Background(), qCtx); err != nil {
		t.Fatal(err)
	}

	// read n4
	l, err := netlink.IpsetList(n4)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Entries) != 1 {
		t.Fatal("no entry")
	}
	e := l.Entries[0]
	if !e.IP.Equal(net.ParseIP("127.0.0.0")) || e.CIDR != 24 {
		t.Fatal()
	}

	// read n6
	l, err = netlink.IpsetList(n6)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Entries) != 1 {
		t.Fatal("no entry")
	}
	e = l.Entries[0]
	if !e.IP.Equal(net.ParseIP("::")) || e.CIDR != 48 {
		t.Fatal()
	}
}
