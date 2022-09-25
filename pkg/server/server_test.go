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

package server

import (
	"crypto/rand"
	"crypto/tls"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/v4/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/v4/pkg/server/http_handler"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
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
	junk := make([]byte, 1200)
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
				echoMsg.Id = uint16(i)
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
		name string
		opts ServerOpts
	}{
		{
			name: "",
			opts: ServerOpts{DNSHandler: dnsHandler},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := getUDPListener(t)
			s := NewServer(tt.opts)
			go func() {
				if err := s.ServeUDP(l); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer s.Close()

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
		opts   ServerOpts
	}{
		{
			name:   "tcp",
			scheme: "tcp",
			opts:   ServerOpts{DNSHandler: dnsHandler},
		},
		{
			name:   "dot with tls config",
			scheme: "tls",
			opts:   ServerOpts{DNSHandler: dnsHandler, TLSConfig: getTLSConfig(t)},
		},
		{
			name:   "dot with cert and key",
			scheme: "tls",
			opts:   ServerOpts{DNSHandler: dnsHandler, Cert: "./testdata/test.test.cert", Key: "./testdata/test.test.key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := getListener(t)
			s := NewServer(tt.opts)
			go func() {
				var err error
				if tt.scheme == "tcp" {
					err = s.ServeTCP(l)
				} else {
					err = s.ServeTLS(l)
				}
				if err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer s.Close()

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
		name string
		opts ServerOpts
	}{
		{
			name: "doh with tls config",
			opts: ServerOpts{
				HttpHandler: httpHandler,
				TLSConfig:   getTLSConfig(t),
			},
		},
		{
			name: "doh with cert and key",
			opts: ServerOpts{
				HttpHandler: httpHandler,
				Cert:        "./testdata/test.test.cert", Key: "./testdata/test.test.key",
			},
		},
		// TODO: Add http test.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := getListener(t)
			s := NewServer(tt.opts)
			go func() {
				if err := s.ServeHTTPS(l); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer s.Close()

			time.Sleep(time.Millisecond * 50)
			u, err := upstream.AddressToUpstream("https://"+l.Addr().String()+"/dns-query", opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}
