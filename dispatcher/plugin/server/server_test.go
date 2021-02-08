//     Copyright (C) 2020-2021, IrineSistiana
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

package server

import (
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server_handler"
	"github.com/miekg/dns"
	"testing"
	"time"
)

var testServerAddr = "127.0.0.123:41234"

func TestUdpServer_ListenAndServe(t *testing.T) {
	tests := []struct {
		name   string
		config *Server
	}{
		{name: "udp1", config: &Server{Protocol: "udp", Addr: testServerAddr}},
		{name: "tcp1", config: &Server{Protocol: "tcp", Addr: testServerAddr}},
		{name: "dot1", config: &Server{Protocol: "dot", Addr: testServerAddr, Cert: "./test.cert", Key: "./test.key"}},
		{name: "doh1 no path", config: &Server{Protocol: "doh", Addr: testServerAddr, Cert: "./test.cert", Key: "./test.key"}},
		{name: "doh2 with path", config: &Server{Protocol: "doh", Addr: testServerAddr, URLPath: "/my-path", Cert: "./test.cert", Key: "./test.key"}},
	}

	for _, tt := range tests {
		if t.Failed() {
			t.FailNow()
			return
		}
		func() {
			sg := NewServerGroup(handler.NewBP("test", PluginType), &server_handler.DummyServerHandler{T: t}, []*Server{tt.config})
			if err := sg.Activate(); err != nil {
				t.Fatal(err)
			}
			defer sg.Shutdown()

			time.Sleep(time.Millisecond * 100)
			opt := upstream.Options{
				Timeout:            time.Second * 2,
				InsecureSkipVerify: true,
			}
			var u upstream.Upstream
			var err error
			switch tt.config.Protocol {
			case "udp":
				u, err = upstream.AddressToUpstream(tt.config.Addr, opt)
			case "tcp":
				u, err = upstream.AddressToUpstream("tcp://"+tt.config.Addr, opt)
			case "dot":
				u, err = upstream.AddressToUpstream("tls://"+tt.config.Addr, opt)
			case "doh":
				u, err = upstream.AddressToUpstream("https://"+tt.config.Addr+tt.config.URLPath, opt)

			// TODO: add http test
			default:
				t.Fatalf("%s: unsupported protocol: %s", tt.name, tt.config.Protocol)
			}
			if err != nil {
				t.Fatalf("%s: %s", tt.name, err)
			}

			for i := 0; i < 50; i++ {
				echoMsg := new(dns.Msg)
				echoMsg.SetQuestion("example.com.", dns.TypeA)
				r, err := u.Exchange(echoMsg)
				if err != nil {
					t.Fatalf("%s: %s", tt.name, err)
				}

				if r.Id != echoMsg.Id {
					t.Fatalf("%s: echoed msg id is not the same", tt.name)
				}
			}
		}()
	}
}
