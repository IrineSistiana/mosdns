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

package blackhole

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"net"
	"testing"
)

func Test_blackhole_Exec(t *testing.T) {
	tests := []struct {
		name         string
		args         *Args
		queryType    uint16
		wantResponse bool
		wantRcode    int
		wantIP       string
	}{
		{"drop response1", &Args{RCode: -1}, dns.TypeA, false, 0, ""},
		{"respond with rcode 2", &Args{RCode: 2}, dns.TypeA, true, 2, ""},
		{"respond with ipv4 1", &Args{IPv4: "127.0.0.1"}, dns.TypeA, true, 0, "127.0.0.1"},
		{"respond with ipv4 2", &Args{IPv4: "127.0.0.1", RCode: 2}, dns.TypeAAAA, true, 2, ""},
		{"respond with ipv6", &Args{IPv6: "127.0.0.1"}, dns.TypeAAAA, true, 0, "127.0.0.1"},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := newBlackHole(coremain.NewBP("test", PluginType, nil, nil), tt.args)
			if err != nil {
				t.Fatal(err)
			}

			q := new(dns.Msg)
			q.SetQuestion("example.com", tt.queryType)
			r := new(dns.Msg)
			r.SetReply(q)
			qCtx := query_context.NewContext(q, nil)
			qCtx.SetResponse(r)

			err = b.Exec(ctx, qCtx, nil)
			if err != nil {
				t.Fatal(err)
			}

			if !tt.wantResponse && qCtx.R() != nil {
				t.Error("response should be dropped")
			}

			if tt.wantResponse {
				if len(tt.wantIP) != 0 {
					wantIP := net.ParseIP(tt.wantIP)
					var gotIP net.IP
					switch tt.queryType {
					case dns.TypeA:
						gotIP = qCtx.R().Answer[0].(*dns.A).A
					case dns.TypeAAAA:
						gotIP = qCtx.R().Answer[0].(*dns.AAAA).AAAA
					}
					if !wantIP.Equal(gotIP) {
						t.Fatalf("ip mismatched, want %v, got %v", wantIP, gotIP)
					}
				}

				if tt.wantRcode != qCtx.R().Rcode {
					t.Fatalf("response should have rcode %d, but got %d", tt.wantRcode, qCtx.R().Rcode)
				}
			}
		})
	}
}
