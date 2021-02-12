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
	"crypto/tls"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"net"
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

func wantErrTest(tb testing.TB, f func() error) {
	errChan := make(chan error, 1)
	go func() {
		errChan <- f()
	}()

	select {
	case err := <-errChan:
		if err == nil {
			tb.Fatal("want f() returns an error, but got nil")
		}
	case <-time.After(time.Second):
		tb.Fatal("f() timeout")
	}
	return
}

func exchangeTest(tb testing.TB, u upstream.Upstream) {
	for i := 0; i < 50; i++ {
		echoMsg := new(dns.Msg)
		echoMsg.SetQuestion("example.com.", dns.TypeA)
		r, err := u.Exchange(echoMsg)
		if err != nil {
			tb.Fatal(err)
		}

		if r.Id != echoMsg.Id {
			tb.Fatal("echoed msg id mismatched")
		}
	}
}

var (
	opt = upstream.Options{
		Timeout:            time.Second * 2,
		InsecureSkipVerify: true,
	}
)

func TestUDPServer(t *testing.T) {
	dnsHandler := &DummyServerHandler{T: t}
	tests := []struct {
		name         string
		server       *Server
		wantStartErr bool
	}{
		{
			name: "udp",
			server: &Server{
				Handler:    dnsHandler,
				Protocol:   ProtocolUDP,
				PacketConn: getUDPListener(t),
			},
			wantStartErr: false,
		},
		{
			name: "nil handler",
			server: &Server{
				Handler:    nil,
				Protocol:   ProtocolUDP,
				PacketConn: getUDPListener(t),
			},
			wantStartErr: true,
		},
		{
			name: "nil packet conn",
			server: &Server{
				Handler:    dnsHandler,
				Protocol:   ProtocolUDP,
				PacketConn: nil,
			},
			wantStartErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantStartErr {
				wantErrTest(t, tt.server.Start)
				return
			}

			go func() {
				if err := tt.server.Start(); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			u, err := upstream.AddressToUpstream(tt.server.PacketConn.LocalAddr().String(), opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}

func TestTCPServer(t *testing.T) {
	dnsHandler := &DummyServerHandler{T: t}
	tests := []struct {
		name         string
		server       *Server
		wantStartErr bool
	}{
		{
			name: "tcp",
			server: &Server{
				Handler:  dnsHandler,
				Protocol: ProtocolTCP,
				Listener: getListener(t),
			},
			wantStartErr: false,
		},
		{
			name: "nil listener",
			server: &Server{
				Handler:  dnsHandler,
				Protocol: ProtocolTCP,
				Listener: nil,
			},
			wantStartErr: true,
		},
		{
			name: "dot",
			server: &Server{
				Handler:   dnsHandler,
				Protocol:  ProtocolDoT,
				Listener:  getListener(t),
				TLSConfig: getTLSConfig(t),
			},
			wantStartErr: false,
		},
		{
			name: "dot no tls config",
			server: &Server{
				Handler:   dnsHandler,
				Protocol:  ProtocolDoT,
				Listener:  getListener(t),
				TLSConfig: nil,
			},
			wantStartErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantStartErr {
				wantErrTest(t, tt.server.Start)
				return
			}

			go func() {
				if err := tt.server.Start(); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			prefix := "tcp"
			if tt.server.Protocol == ProtocolDoT {
				prefix = "tls"
			}
			u, err := upstream.AddressToUpstream(prefix+"://"+tt.server.Listener.Addr().String(), opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}

func TestDoHServer(t *testing.T) {
	dnsHandler := &DummyServerHandler{T: t}
	tests := []struct {
		name         string
		server       *Server
		wantStartErr bool
	}{
		{
			name: "doh",
			server: &Server{
				Handler:   dnsHandler,
				Protocol:  ProtocolDoH,
				Listener:  getListener(t),
				URLPath:   "/dns-query",
				TLSConfig: getTLSConfig(t),
			},
			wantStartErr: false,
		},
		{
			name: "nil listener",
			server: &Server{
				Handler:   dnsHandler,
				Protocol:  ProtocolDoH,
				Listener:  nil,
				TLSConfig: getTLSConfig(t),
			},
			wantStartErr: true,
		},
		{
			name: "nil tls config",
			server: &Server{
				Handler:   dnsHandler,
				Protocol:  ProtocolDoH,
				Listener:  getListener(t),
				TLSConfig: nil,
			},
			wantStartErr: true,
		},
		// TODO: Add http test.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantStartErr {
				wantErrTest(t, tt.server.Start)
				return
			}

			go func() {
				if err := tt.server.Start(); err != ErrServerClosed {
					t.Error(err)
				}
			}()
			defer tt.server.Close()

			u, err := upstream.AddressToUpstream("https://"+tt.server.Listener.Addr().String()+tt.server.URLPath, opt)
			if err != nil {
				t.Fatal(err)
			}
			exchangeTest(t, u)
		})
	}
}
