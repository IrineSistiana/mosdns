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

package cache

import (
	"bytes"
	"net"
	"reflect"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
	"strconv"
	"testing"
	"time"
)

func Test_cachePlugin_Dump(t *testing.T) {
	c := NewCache(&Args{Size: 16 * dumpBlockSize}, Opts{}) // Big enough to create dump fragments.

	resp := new(dns.Msg)
	resp.SetQuestion("test.", dns.TypeA)

	now := time.Now()
	hourLater := now.Add(time.Hour)
	v := &item{
		resp:           resp,
		storedTime:     now,
		expirationTime: hourLater,
	}

	// Fill the cache
	for i := 0; i < 32*dumpBlockSize; i++ {
		c.backend.Store(key(strconv.Itoa(i)), v, hourLater)
	}

	buf := new(bytes.Buffer)
	enw, err := c.writeDump(buf)
	if err != nil {
		t.Fatal(err)
	}
	enr, err := c.readDump(buf)
	if err != nil {
		t.Fatal(err)
	}

	if enw != enr {
		t.Fatalf("read err, wrote %d entries, read %d", enw, enr)
	}
}

func TestGetMsgKey_EDNS(t *testing.T) {
	newContext := func(do bool, ecsIP string) *query_context.Context {
		q := new(dns.Msg)
		q.SetQuestion("example.com.", dns.TypeA)
		q.SetEdns0(1232, do)
		if ecsIP != "" {
			q.IsEdns0().Option = append(q.IsEdns0().Option, &dns.EDNS0_SUBNET{
				Code:          dns.EDNS0SUBNET,
				Family:        1,
				SourceNetmask: 24,
				Address:       net.ParseIP(ecsIP).To4(),
			})
		}
		return query_context.NewContext(q)
	}

	plain := newContext(false, "")
	withDO := newContext(true, "")
	withECS1 := newContext(false, "192.0.2.1")
	withECS2 := newContext(false, "192.0.3.1")

	// NewContext replaces the working OPT, so this specifically protects
	// against losing the client's DO bit and ECS before downstream EDNS
	// handlers have run.
	if plain.Q().IsEdns0().Do() || withDO.Q().IsEdns0().Do() {
		t.Fatal("working query unexpectedly retained the client DO bit")
	}

	keys := map[string]struct{}{
		getMsgKey(plain):    {},
		getMsgKey(withDO):   {},
		getMsgKey(withECS1): {},
		getMsgKey(withECS2): {},
	}
	if len(keys) != 4 {
		t.Fatal("different EDNS queries must not share a cache key")
	}
}

func TestGetMsgKey_EffectiveQueryEDNS(t *testing.T) {
	newContext := func(ecsIP string) *query_context.Context {
		q := new(dns.Msg)
		q.SetQuestion("example.com.", dns.TypeA)
		ctx := query_context.NewContext(q)
		ctx.QOpt().Option = append(ctx.QOpt().Option, &dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Family:        1,
			SourceNetmask: 24,
			Address:       net.ParseIP(ecsIP).To4(),
		})
		return ctx
	}

	if getMsgKey(newContext("198.51.100.1")) == getMsgKey(newContext("198.51.101.1")) {
		t.Fatal("ECS inserted before cache must be part of the cache key")
	}
}

func TestCachedResponseRestoresUpstreamOPT(t *testing.T) {
	backend := NewCache(&Args{Size: 16}, Opts{}).backend
	resp := new(dns.Msg)
	resp.SetQuestion("example.com.", dns.TypeA)
	resp.Answer = append(resp.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP("192.0.2.1").To4(),
	})
	upstreamOpt := &dns.OPT{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT}}
	upstreamOpt.Option = append(upstreamOpt.Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "cached"})

	if !saveRespToCache("test", resp, upstreamOpt, backend, 0) {
		t.Fatal("response was not cached")
	}
	got, lazy := getRespFromCache("test", backend, false, 0)
	if lazy || got == nil {
		t.Fatal("cached response was not returned")
	}
	gotOpt := got.IsEdns0()
	if gotOpt == nil || !reflect.DeepEqual(gotOpt.Option, upstreamOpt.Option) {
		t.Fatalf("cached upstream OPT = %#v, want %#v", gotOpt, upstreamOpt)
	}

	ctx := query_context.NewContext(new(dns.Msg))
	ctx.SetResponse(got)
	if gotOpt := ctx.UpstreamOpt(); gotOpt == nil || !reflect.DeepEqual(gotOpt.Option, upstreamOpt.Option) {
		t.Fatalf("restored upstream OPT = %#v, want %#v", gotOpt, upstreamOpt)
	}
}
