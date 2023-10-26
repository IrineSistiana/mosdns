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

package upstream

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/miekg/dns"
)

func newUDPTestServer(t testing.TB, handler dns.Handler) (addr string, shutdownFunc func()) {
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpAddr := udpConn.LocalAddr().String()
	udpServer := dns.Server{
		PacketConn: udpConn,
		Handler:    handler,
	}
	go udpServer.ActivateAndServe()
	return udpAddr, func() {
		udpServer.Shutdown()
	}
}

func newTCPTestServer(t testing.TB, handler dns.Handler) (addr string, shutdownFunc func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcpAddr := l.Addr().String()
	tcpServer := dns.Server{
		Listener:      l,
		Handler:       handler,
		MaxTCPQueries: -1,
	}
	go tcpServer.ActivateAndServe()
	return tcpAddr, func() {
		tcpServer.Shutdown()
	}
}

func newDoTTestServer(t testing.TB, handler dns.Handler) (addr string, shutdownFunc func()) {
	serverName := "test"
	cert, err := utils.GenerateCertificate(serverName)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig := new(tls.Config)
	tlsConfig.Certificates = []tls.Certificate{cert}
	tlsListener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	doTAddr := tlsListener.Addr().String()
	doTServer := dns.Server{
		Net:           "tcp-tls",
		Listener:      tlsListener,
		TLSConfig:     tlsConfig,
		Handler:       handler,
		MaxTCPQueries: -1,
	}
	go doTServer.ActivateAndServe()
	return doTAddr, func() {
		doTServer.Shutdown()
	}
}

type newTestServerFunc func(t testing.TB, handler dns.Handler) (addr string, shutdownFunc func())

var m = map[string]newTestServerFunc{
	"udp": newUDPTestServer,
	"tcp": newTCPTestServer,
	"tls": newDoTTestServer,
}

func Test_fastUpstream(t *testing.T) {

	// TODO: add test for doh
	// TODO: add test for socks5

	// server config
	for scheme, f := range m {
		for _, bigMsg := range [...]bool{true, false} {
			for _, latency := range [...]time.Duration{0, time.Millisecond * 10} {

				// client specific
				for _, idleTimeout := range [...]time.Duration{0, time.Second} {

					testName := fmt.Sprintf(
						"test: protocol: %s, bigMsg: %v, latency: %s, getIdleTimeout: %s",
						scheme,
						bigMsg,
						latency,
						idleTimeout,
					)

					t.Run(testName, func(t *testing.T) {
						addr, shutdownServer := f(t, &vServer{
							latency: latency,
							bigMsg:  bigMsg,
						})
						defer shutdownServer()
						u, err := NewUpstream(
							scheme+"://"+addr,
							Opt{
								IdleTimeout: time.Second,
								TLSConfig:   &tls.Config{InsecureSkipVerify: true},
							},
						)
						if err != nil {
							t.Fatal(err)
						}

						if err := testUpstream(u); err != nil {
							t.Fatal(err)
						}
					})
				}
			}
		}

	}
}

func testUpstream(u Upstream) error {
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

	for i := uint16(0); i < 10; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()

			q := new(dns.Msg)
			q.SetQuestion("example.com.", dns.TypeA)
			q.Id = i
			queryPayload, err := q.Pack()
			if err != nil {
				logErr(err)
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r, err := u.ExchangeContext(ctx, queryPayload)
			if err != nil {
				logErr(err)
				return
			}

			resp := new(dns.Msg)
			err = resp.Unpack(*r)
			if err != nil {
				logErr(err)
				return
			}
			if q.Id != resp.Id {
				logErr(dns.ErrId)
				return
			}
			if !resp.Response {
				logErr(fmt.Errorf("resp is not a resp bit"))
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
	bigMsg  bool // with 1kb padding
}

var padding = make([]byte, 1024)

func (s *vServer) ServeDNS(w dns.ResponseWriter, q *dns.Msg) {
	r := new(dns.Msg)
	r.SetReply(q)
	if s.bigMsg {
		r.SetEdns0(dns.MaxMsgSize, false)
		opt := r.IsEdns0()
		opt.Option = append(opt.Option, &dns.EDNS0_PADDING{Padding: padding})
	}

	time.Sleep(s.latency)
	w.WriteMsg(r)
}
