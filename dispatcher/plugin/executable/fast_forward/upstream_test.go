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

package fastforward

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"net"
	"sync"
	"testing"
	"time"
)

var (
	dummyIP     = net.IPv4(1, 2, 3, 4)
	dummyServer = &vServer{ip: dummyIP, latency: 0}

	testLogger = mlog.NewPluginLogger("test")
)

func Test_fastUpstream(t *testing.T) {
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpAddr := udpConn.LocalAddr().String()
	udpServer := dns.Server{Net: "udp", PacketConn: udpConn, Handler: dummyServer}
	go udpServer.ActivateAndServe()
	defer udpServer.Shutdown()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcpAddr := l.Addr().String()
	tcpServer := dns.Server{Net: "udp", Listener: l, Handler: dummyServer}
	go tcpServer.ActivateAndServe()
	defer tcpServer.Shutdown()

	serverName := "test"
	cert, err := utils.GenerateCertificate(serverName)
	tlsConfig := new(tls.Config)
	tlsConfig.Certificates = []tls.Certificate{cert}
	tlsListener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	tlsAddr := tlsListener.Addr().String()
	tlsServer := dns.Server{Net: "tcp-tls", Listener: tlsListener, TLSConfig: tlsConfig, Handler: dummyServer}
	go tlsServer.ActivateAndServe()
	defer tlsServer.Shutdown()

	conf := []*UpstreamConfig{
		{
			Protocol:    "",
			Addr:        udpAddr,
			IdleTimeout: 0,
		},
		{
			Protocol:    "udp",
			Addr:        udpAddr,
			IdleTimeout: 5,
		},
		{
			Protocol:    "tcp",
			Addr:        tcpAddr,
			IdleTimeout: 0,
		},
		{
			Protocol:    "tcp",
			Addr:        tcpAddr,
			IdleTimeout: 5,
		},
		{
			Protocol:           "dot",
			Addr:               tlsAddr,
			IdleTimeout:        0,
			ServerName:         serverName,
			InsecureSkipVerify: true,
		},
		{
			Protocol:           "dot",
			Addr:               tlsAddr,
			IdleTimeout:        5,
			ServerName:         serverName,
			InsecureSkipVerify: true,
		},
	}

	// TODO: add test for doh
	// TODO: add test for socks5

	for i, c := range conf {
		u, err := newFastUpstream(c, testLogger, nil)
		if err != nil {
			t.Fatalf("test %d: %v", i, err)
		}
		if err := testUpstream(u); err != nil {
			t.Fatalf("test %d: %v", i, err)
		}
	}
}

func testUpstream(u upstream.Upstream) error {
	wg := sync.WaitGroup{}
	errs := make([]error, 0)
	errsLock := sync.Mutex{}
	logErr := func(err error) {
		errsLock.Lock()
		errs = append(errs, err)
		errsLock.Unlock()
	}
	errsToString := func() string {
		s := fmt.Sprintf("%d err(s) occured during the test: ", len(errs))
		for i := range errs {
			s = s + errs[i].Error() + "|"
		}
		return s
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			q := new(dns.Msg)
			q.SetQuestion("example.com.", dns.TypeA)

			r, err := u.Exchange(q)
			if err != nil {
				logErr(err)
				return
			}
			if !r.Answer[0].(*dns.A).A.Equal(dummyIP) {
				logErr(errors.New("data corrupted"))
				return
			}
		}()
	}
	wg.Wait()
	if len(errs) != 0 {
		return errors.New(errsToString())
	}
	return nil
}

type vServer struct {
	latency time.Duration
	ip      net.IP
}

func (s *vServer) ServeDNS(w dns.ResponseWriter, q *dns.Msg) {
	name := q.Question[0].Name
	r := new(dns.Msg)
	r.SetReply(q)
	var rr dns.RR
	hdr := dns.RR_Header{
		Name:     name,
		Class:    dns.ClassINET,
		Ttl:      300,
		Rdlength: 0,
	}

	hdr.Rrtype = dns.TypeA

	rr = &dns.A{Hdr: hdr, A: s.ip}
	r.Answer = append(r.Answer, rr)

	time.Sleep(s.latency)
	w.WriteMsg(r)
}
