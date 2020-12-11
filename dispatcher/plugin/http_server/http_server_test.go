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

package httpserver

import (
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"reflect"
	"testing"
	"time"
)

func TestHttp_server(t *testing.T) {
	args := &Args{
		Listen: "127.0.0.1:51234",
		Path:   "/my-path",
		Cert:   "./http.test.cert",
		Key:    "./http.test.key",
	}

	echoMsg := new(dns.Msg)
	echoMsg.SetQuestion("example.com.", dns.TypeA)

	s, err := startNewServer("test", args)
	s.dnsHandler = &handler.DummyServerHandler{T: t, EchoMsg: echoMsg}
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()

	u, err := upstream.AddressToUpstream("https://127.0.0.1:51234/my-path", upstream.Options{
		Timeout:            time.Second * 2,
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 50; i++ {
		r, err := u.Exchange(echoMsg)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(r, echoMsg) {
			t.Fatal("echoed msg is not the same")
		}
	}
}
