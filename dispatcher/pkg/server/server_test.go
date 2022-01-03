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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server/http_handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
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
	opt = &upstream.Options{
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
			name:   "",
			server: &Server{DNSHandler: dnsHandler},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := getUDPListener(t)
			go func() {
				if err := tt.server.ServeUDP(l); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			time.Sleep(time.Millisecond * 50)
			addr := l.LocalAddr().String()
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

func TestTCPDoTServer(t *testing.T) {
	dnsHandler := &dns_handler.DummyServerHandler{T: t}
	tests := []struct {
		name   string
		scheme string
		server *Server
	}{
		{
			name:   "tcp",
			scheme: "tcp",
			server: &Server{DNSHandler: dnsHandler},
		},
		{
			name:   "dot with tls config",
			scheme: "tls",
			server: &Server{DNSHandler: dnsHandler, TLSConfig: getTLSConfig(t)},
		},
		{
			name:   "dot with cert and key",
			scheme: "tls",
			server: &Server{DNSHandler: dnsHandler, Cert: "./testdata/test.test.cert", Key: "./testdata/test.test.key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := getListener(t)
			go func() {
				var err error
				if tt.scheme == "tcp" {
					err = tt.server.ServeTCP(l)
				} else {
					err = tt.server.ServeTLS(l)
				}
				if err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			time.Sleep(time.Millisecond * 50)
			addr := l.Addr().String()
			c, err := net.Dial("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			writeJunkData(c)

			u, err := upstream.AddressToUpstream(tt.scheme+"://"+addr, opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}

func TestDoHServer(t *testing.T) {
	dnsHandler := &dns_handler.DummyServerHandler{T: t}
	httpHandler := &http_handler.Handler{
		DNSHandler: dnsHandler,
		Path:       "/dns-query",
	}
	tests := []struct {
		name   string
		server *Server
	}{
		{
			name: "doh with tls config",
			server: &Server{
				HttpHandler: httpHandler,
				TLSConfig:   getTLSConfig(t),
			},
		},
		{
			name: "doh with cert and key",
			server: &Server{
				HttpHandler: httpHandler,
				Cert:        "./testdata/test.test.cert", Key: "./testdata/test.test.key",
			},
		},
		// TODO: Add http test.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := getListener(t)
			go func() {
				if err := tt.server.ServeHTTPS(l); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			time.Sleep(time.Millisecond * 50)
			u, err := upstream.AddressToUpstream("https://"+l.Addr().String()+"/dns-query", opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}
