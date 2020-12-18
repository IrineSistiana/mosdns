//     Copyright (C) 2020, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package ecs

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
	"net"
	"reflect"
	"testing"
)

func Test_ecs(t *testing.T) {
	argsStr := `
auto: false
force_overwrite: true
mask4: 24
mask6: 32
ipv4: 1.2.3.4
ipv6: '2001:dd8:1a::'
`
	args := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(argsStr), &args); err != nil {
		t.Fatal(err)
	}

	// test Init
	p, err := Init("test", args)
	if err != nil {
		t.Fatal(err)
	}

	fpw := p.(*handler.FunctionalPluginWrapper)
	ecs := fpw.Functional.(*ecsPlugin)
	ctx := context.Background()
	from := utils.NewNetAddr("test", "192.168.0.1:0")

	testFunc := func(presetECS bool) {
		typ := []uint16{dns.TypeA, dns.TypeAAAA}
		wantECS := []*dns.EDNS0_SUBNET{ecs.ipv4, ecs.ipv6}
		otherECS := newEDNS0Subnet(net.IPv4(1, 2, 3, 4), 32, false)

		for i := 0; i < 2; i++ {
			m := new(dns.Msg)
			m.SetQuestion("example.com.", typ[i])
			if getMsgECS(m) != nil {
				t.FailNow()
			}

			if presetECS {
				setECS(m, otherECS)
				if getMsgECS(m) != otherECS {
					t.FailNow()
				}
			}

			err = fpw.Do(ctx, &handler.Context{Q: m, From: from})
			if err != nil {
				t.Fatal(err)
			}

			e := getMsgECS(m)
			if !reflect.DeepEqual(e, wantECS[i]) {
				t.Fatal("ecs not equal")
			}
		}
	}
	// test appending ecs to non-ecs msg
	testFunc(false)

	// test overwrite ecs msg
	testFunc(true)
}

func Test_ecs_auto(t *testing.T) {

	p, err := newPlugin("test", &Args{
		Auto:           true,
		ForceOverwrite: true,
		Mask4:          24,
		Mask6:          32,
	})
	if err != nil {
		t.Fatal(err)
	}
	fpw := p.(*handler.FunctionalPluginWrapper)

	testFunc := func(presetECS bool) {
		typ := []uint16{dns.TypeA, dns.TypeAAAA}
		from := []net.Addr{
			utils.NewNetAddr("test", "192.168.0.1:0"),
			utils.NewNetAddr("test", "[2001:0db8::]:0"),
		}
		wantECS := []*dns.EDNS0_SUBNET{
			newEDNS0Subnet(net.ParseIP("192.168.0.1").To4(), 24, false),
			newEDNS0Subnet(net.ParseIP("2001:0db8::").To16(), 32, true)}
		otherECS := newEDNS0Subnet(net.IPv4(1, 2, 3, 4), 32, false)

		for i := 0; i < 2; i++ {
			m := new(dns.Msg)
			m.SetQuestion("example.com.", typ[i])
			if getMsgECS(m) != nil {
				t.FailNow()
			}

			if presetECS {
				setECS(m, otherECS)
				if getMsgECS(m) != otherECS {
					t.FailNow()
				}
			}

			err = fpw.Do(context.Background(), &handler.Context{Q: m, From: from[i]})
			if err != nil {
				t.Fatal(err)
			}

			e := getMsgECS(m)
			if !reflect.DeepEqual(e, wantECS[i]) {
				t.Fatal("ecs not equal")
			}
		}
	}

	// test appending ecs to non-ecs msg
	testFunc(false)

	// test overwrite ecs msg
	testFunc(true)
}

func Test_remove_ecs(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	ecs := newEDNS0Subnet(net.IPv4(1, 2, 3, 4), 32, false)
	setECS(m, ecs)

	p := &noECS{}
	err := p.Do(context.Background(), &handler.Context{Q: m})
	if err != nil {
		t.Fatal(err)
	}
	if e := getMsgECS(m); e != nil {
		t.FailNow()
	}
}
