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

package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

func (s *Server) serveDoH(noTLS bool) error {
	l, err := net.Listen("tcp", s.Config.Addr)
	if err != nil {
		return err
	}

	s.l.Lock()
	s.listener = l
	s.l.Unlock()
	defer func() {
		s.l.Lock()
		l.Close()
		s.l.Unlock()
	}()

	httpServer := &http.Server{
		Handler:        s,
		ReadTimeout:    time.Second * 5,
		WriteTimeout:   time.Second * 5,
		IdleTimeout:    s.idleTimeout,
		MaxHeaderBytes: 2048,
	}

	if noTLS {
		err = httpServer.Serve(l)
	} else {
		if len(s.Config.Cert) == 0 || len(s.Config.Key) == 0 {
			return errors.New("doh server needs cert and key")
		}
		err = httpServer.ServeTLS(l, s.Config.Cert, s.Config.Key)
	}
	if err != nil && s.isDone() {
		err = nil
	}
	return nil

}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if len(s.Config.URLPath) != 0 && req.URL.Path != s.Config.URLPath {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	q, err := getMsgFromReq(req)
	if err != nil {
		s.Logger.Warnf("invalid request from %s: %v", req.RemoteAddr, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	qCtx := handler.NewContext(q)
	qCtx.From = httpAddr(req.RemoteAddr)

	responseWriter := &httpDnsRespWriter{httpRespWriter: w}
	ctx, cancel := context.WithTimeout(req.Context(), s.queryTimeout)
	defer cancel()
	s.Handler.ServeDNS(ctx, qCtx, responseWriter)
}

func getMsgFromReq(req *http.Request) (*dns.Msg, error) {
	var b []byte
	var err error
	switch req.Method {
	case http.MethodGet:
		s := req.URL.Query().Get("dns")
		if len(s) == 0 {
			return nil, fmt.Errorf("no dns parameter in url %s", req.RequestURI)
		}
		b, err = base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("failed to decode url %s: %w", req.RequestURI, err)
		}
	case http.MethodPost:
		b, err = ioutil.ReadAll(io.LimitReader(req.Body, dns.MaxMsgSize))
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}

	q := new(dns.Msg)
	if err := q.Unpack(b); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	return q, nil
}

type httpAddr string

func (h httpAddr) Network() string {
	return "http"
}

func (h httpAddr) String() string {
	return string(h)
}

type httpDnsRespWriter struct {
	httpRespWriter http.ResponseWriter
}

func (h *httpDnsRespWriter) Write(m *dns.Msg) (n int, err error) {
	return utils.WriteMsgToUDP(h.httpRespWriter, m)
}
