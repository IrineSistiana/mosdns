package ecs

import (
	"context"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
	"net"
	"testing"
)

func Test_ecsPlugin(t *testing.T) {
	tests := []struct {
		name       string
		args       Args
		qtype      uint16
		qHasEDNS0  bool
		qHasECS    string
		clientAddr string
		wantAddr   string
		rWantEDNS0 bool
		rWantECS   bool
	}{
		{"edns0 contingency", Args{Auto: true}, dns.TypeA, false, "", "1.0.0.0", "1.0.0.0", false, false},
		{"edns0 contingency", Args{Auto: true}, dns.TypeA, true, "", "1.0.0.0", "1.0.0.0", true, false},
		{"ecs contingency", Args{Auto: true}, dns.TypeA, true, "", "1.0.0.0", "1.0.0.0", true, false},
		{"ecs contingency", Args{Auto: true}, dns.TypeA, true, "1.0.0.0", "1.0.0.0", "1.0.0.0", true, true},

		{"auto", Args{Auto: true}, dns.TypeA, false, "", "1.0.0.0", "1.0.0.0", false, false},
		{"auto", Args{Auto: true}, dns.TypeA, false, "", "", "", false, false},

		{"overwrite off", Args{Auto: true}, dns.TypeA, true, "1.2.3.4", "1.0.0.0", "1.2.3.4", true, true},
		{"overwrite on", Args{Auto: true, ForceOverwrite: true}, dns.TypeA, true, "1.2.3.4", "1.0.0.0", "1.0.0.0", true, true},

		{"preset v4", Args{IPv4: "1.2.3.4"}, dns.TypeA, false, "", "", "1.2.3.4", false, false},
		{"preset v6", Args{IPv6: "::1"}, dns.TypeA, false, "", "", "::1", false, false},
		{"preset both", Args{IPv4: "1.2.3.4", IPv6: "::1"}, dns.TypeA, false, "", "", "1.2.3.4", false, false},
		{"preset both", Args{IPv4: "1.2.3.4", IPv6: "::1"}, dns.TypeAAAA, false, "", "", "::1", false, false},
	}
	for _, tt := range tests {
		p, err := newPlugin(handler.NewBP("ecs", PluginType), &tt.args)
		if err != nil {
			t.Fatal(err)
		}

		t.Run(tt.name, func(t *testing.T) {
			q := new(dns.Msg)
			q.SetQuestion(".", tt.qtype)
			r := new(dns.Msg)
			r.SetReply(q)

			if tt.qHasEDNS0 {
				optQ := dnsutils.UpgradeEDNS0(q)
				optR := dnsutils.UpgradeEDNS0(r)

				if len(tt.qHasECS) > 0 {
					ip := net.ParseIP(tt.qHasECS)
					if ip == nil {
						t.Fatal("invalid ip")
					}
					dnsutils.AddECS(optR, dnsutils.NewEDNS0Subnet(net.IPv6loopback, 24, false), true)
					dnsutils.AddECS(optQ, dnsutils.NewEDNS0Subnet(ip, 24, false), true)
				}
			}

			var clientAddr net.Addr
			if len(tt.clientAddr) > 0 {
				ip := net.ParseIP(tt.clientAddr)
				if ip == nil {
					t.Fatal("invalid ip")
				}
				clientAddr = &net.TCPAddr{
					IP:   ip,
					Port: 0,
				}
			}
			qCtx := handler.NewContext(q, clientAddr)

			next := handler.WrapExecutable(&handler.DummyExecutablePlugin{
				BP:    handler.NewBP("next", PluginType),
				WantR: r,
			})
			if err := p.Exec(context.Background(), qCtx, next); err != nil {
				t.Fatal(err)
			}

			var qECS net.IP
			e := dnsutils.GetMsgECS(q)
			if e != nil {
				qECS = e.Address
			}
			wantAddr := net.ParseIP(tt.wantAddr)
			if !qECS.Equal(wantAddr) {
				t.Fatalf("want addr %v, got %v", tt.wantAddr, qECS)
			}

			if res := dnsutils.GetMsgECS(qCtx.R()) != nil; res != tt.rWantECS {
				t.Fatalf("want rWantECS %v, got %v", tt.rWantECS, res)
			}
			if res := qCtx.R().IsEdns0() != nil; res != tt.rWantEDNS0 {
				t.Fatalf("want rWantEDNS0 %v, got %v", tt.rWantEDNS0, res)
			}
		})
	}
}
