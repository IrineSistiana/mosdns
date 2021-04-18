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
	"crypto/rand"
	"crypto/tls"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server/http_handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"net"
	"sync"
	"testing"
	"time"
)

func getListener(tb testing.TB) net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	return l
}

func getUDPListener(tb testing.TB) net.PacketConn {
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	return l
}

func getTLSConfig(tb testing.TB) *tls.Config {
	tlsConfig := new(tls.Config)
	cert, err := utils.GenerateCertificate("test")
	if err != nil {
		tb.Fatal()
	}
	tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
	return tlsConfig
}

func writeJunkData(c net.Conn) {
	junk := make([]byte, dnsutils.IPv4UdpMaxPayload)
	rand.Read(junk)
	c.Write(junk)
}

func exchangeTest(tb testing.TB, u upstream.Upstream) {
	wg := new(sync.WaitGroup)
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				echoMsg := new(dns.Msg)
				echoMsg.SetQuestion("example.com.", dns.TypeA)
				r, err := u.Exchange(echoMsg)
				if err != nil {
					tb.Error(err)
					return
				}

				if r.Id != echoMsg.Id {
					tb.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

var (
	opt = upstream.Options{
		Timeout:            time.Second * 2,
		InsecureSkipVerify: true,
	}
)

func TestUDPServer(t *testing.T) {
	dnsHandler := &dns_handler.DummyServerHandler{T: t}
	tests := []struct {
		name   string
		server *Server
	}{
		{
			name:   "with PacketConn",
			server: NewServer("udp", "", WithHandler(dnsHandler), WithPacketConn(getUDPListener(t))),
		},
		{
			name:   "with addr",
			server: NewServer("udp", "127.0.0.1:0", WithHandler(dnsHandler)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go func() {
				if err := tt.server.Start(); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			time.Sleep(time.Millisecond * 50)
			addr := tt.server.getPacketConn().LocalAddr().String()
			c, err := net.Dial("udp", addr)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			writeJunkData(c)

			u, err := upstream.AddressToUpstream(addr, opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}

func TestTCPServer(t *testing.T) {
	dnsHandler := &dns_handler.DummyServerHandler{T: t}
	tests := []struct {
		name   string
		server *Server
	}{
		{
			name: "tcp with listener",
			server: NewServer(
				"tcp",
				"",
				WithHandler(dnsHandler),
				WithListener(getListener(t)),
			),
		},
		{
			name: "tcp with addr",
			server: NewServer(
				"tcp",
				"127.0.0.1:0",
				WithHandler(dnsHandler),
			),
		},
		{
			name: "dot with listener",
			server: NewServer(
				"dot",
				"",
				WithHandler(dnsHandler),
				WithListener(getListener(t)),
				WithTLSConfig(getTLSConfig(t)),
			),
		},
		{
			name: "dot with cert and key",
			server: NewServer(
				"dot",
				"",
				WithHandler(dnsHandler),
				WithListener(getListener(t)),
				WithCertificate("./testdata/test.test.cert", "./testdata/test.test.key"),
			),
		},
		{
			name: "dot with addr",
			server: NewServer(
				"dot",
				"127.0.0.1:0",
				WithHandler(dnsHandler),
				WithTLSConfig(getTLSConfig(t)),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go func() {
				if err := tt.server.Start(); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			time.Sleep(time.Millisecond * 50)
			addr := tt.server.getListener().Addr().String()
			c, err := net.Dial("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			writeJunkData(c)

			prefix := "tcp"
			if tt.server.protocol == "dot" {
				prefix = "tls"
			}
			u, err := upstream.AddressToUpstream(prefix+"://"+addr, opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}

func TestDoHServer(t *testing.T) {
	dnsHandler := &dns_handler.DummyServerHandler{T: t}
	tests := []struct {
		name   string
		server *Server
	}{
		{
			name: "doh with listener",
			server: NewServer(
				"doh",
				"",
				WithHttpHandler(http_handler.NewHandler(dnsHandler, http_handler.WithPath("/dns-query"))),
				WithListener(getListener(t)),
				WithTLSConfig(getTLSConfig(t)),
			),
		},
		{
			name: "doh with address",
			server: NewServer(
				"doh",
				"127.0.0.1:0",
				WithHttpHandler(http_handler.NewHandler(dnsHandler, http_handler.WithPath("/dns-query"))),
				WithTLSConfig(getTLSConfig(t)),
			),
		},
		{
			name: "doh with cert and key",
			server: NewServer(
				"doh",
				"",
				WithHttpHandler(http_handler.NewHandler(dnsHandler, http_handler.WithPath("/dns-query"))),
				WithListener(getListener(t)),
				WithCertificate("./testdata/test.test.cert", "./testdata/test.test.key"),
			),
		},
		{
			name: "doh without url path",
			server: NewServer(
				"doh",
				"",
				WithHttpHandler(http_handler.NewHandler(dnsHandler)),
				WithListener(getListener(t)),
				WithCertificate("./testdata/test.test.cert", "./testdata/test.test.key"),
			),
		},
		// TODO: Add http test.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go func() {
				if err := tt.server.Start(); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			time.Sleep(time.Millisecond * 50)
			u, err := upstream.AddressToUpstream("https://"+tt.server.getListener().Addr().String()+"/dns-query", opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}
