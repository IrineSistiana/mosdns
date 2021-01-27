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
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

func (sg *ServerGroup) startDoH(conf *ServerConfig, noTLS bool) error {
	if !noTLS && (len(conf.Cert) == 0 || len(conf.Key) == 0) { // no cert
		return errors.New("doh server needs cert and key")
	}

	l, err := net.Listen("tcp", conf.Addr)
	if err != nil {
		return err
	}

	sg.L().Info("doh server started", zap.Stringer("addr", l.Addr()))
	sg.listener[l] = struct{}{}

	httpServer := &http.Server{
		Handler: &dohHandler{
			s:    sg,
			conf: conf,
		},
		ReadTimeout:    time.Second * 5,
		WriteTimeout:   time.Second * 5,
		IdleTimeout:    conf.idleTimeout,
		MaxHeaderBytes: 2048,
	}

	go func() {
		var err error
		if noTLS {
			err = httpServer.Serve(l)
		} else {
			err = httpServer.ServeTLS(l, conf.Cert, conf.Key)
		}
		if err != nil {
			if sg.isClosed() {
				return
			}
			sg.errChan <- err
		}
	}()

	return nil

}

type dohHandler struct {
	s    *ServerGroup
	conf *ServerConfig
}

func (h *dohHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if len(h.conf.URLPath) != 0 && req.URL.Path != h.conf.URLPath {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	q, err := getMsgFromReq(req)
	if err != nil {
		h.s.L().Warn("invalid request", zap.String("from", req.RemoteAddr), zap.String("url", req.RequestURI), zap.String("method", req.Method), zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	remoteAddr := req.RemoteAddr
	if len(h.conf.GetUserIPFromHeader) != 0 {
		if ip := req.Header.Get(h.conf.GetUserIPFromHeader); len(ip) != 0 {
			remoteAddr = ip + ":0"
		}
	}

	qCtx := handler.NewContext(q, utils.NewNetAddr(remoteAddr, req.URL.Scheme))
	qCtx.SetTCPClient(true)
	h.s.L().Debug("new query", qCtx.InfoField(), zap.String("from", req.RemoteAddr))

	responseWriter := &httpDnsRespWriter{httpRespWriter: w}
	ctx, cancel := context.WithTimeout(req.Context(), h.conf.queryTimeout)
	defer cancel()
	h.s.handler.ServeDNS(ctx, qCtx, responseWriter)
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

type httpDnsRespWriter struct {
	httpRespWriter http.ResponseWriter
}

func (h *httpDnsRespWriter) Write(m *dns.Msg) (n int, err error) {
	return utils.WriteMsgToUDP(h.httpRespWriter, m)
}
