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

package server_handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/pool"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net/http"
	"time"
)

type DoHHandler struct {
	Logger              *zap.Logger      // It cannot be nil.
	URLPath             string           // If empty, DoHHandler will not check request's path.
	GetUserIPFromHeader string           // Get client ip from http header, e.g. for nginx, X-Forwarded-For.
	QueryTimeout        time.Duration    // This should > 0.
	DNSHandler          DNSServerHandler // DNS handler for incoming requests.
}

func (h *DoHHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if len(h.URLPath) != 0 && req.URL.Path != h.URLPath {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	q, err := GetMsgFromReq(req)
	if err != nil {
		h.Logger.Warn("invalid request", zap.String("from", req.RemoteAddr), zap.String("url", req.RequestURI), zap.String("method", req.Method), zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	qCtx := handler.NewContext(q, GetClientIPFromReq(req, h.GetUserIPFromHeader))
	qCtx.SetTCPClient(true)
	h.Logger.Debug("new query", qCtx.InfoField(), zap.String("from", req.RemoteAddr))

	responseWriter := &httpDnsRespWriter{httpRespWriter: w}
	ctx, cancel := context.WithTimeout(req.Context(), h.QueryTimeout)
	defer cancel()
	h.DNSHandler.ServeDNS(ctx, qCtx, responseWriter)
}

func GetClientIPFromReq(req *http.Request, checkHeader string) *utils.NetAddr {
	remoteAddr := req.RemoteAddr
	if len(checkHeader) != 0 {
		if ip := req.Header.Get(checkHeader); len(ip) != 0 {
			remoteAddr = ip + ":0"
		}
	}
	return utils.NewNetAddr(remoteAddr, req.URL.Scheme)
}

func GetMsgFromReq(req *http.Request) (*dns.Msg, error) {
	var b []byte
	var err error
	switch req.Method {
	case http.MethodGet:
		s := req.URL.Query().Get("dns")
		if len(s) == 0 {
			return nil, errors.New("no dns parameter")
		}
		msgSize := base64.RawURLEncoding.DecodedLen(len(s))
		if msgSize > dns.MaxMsgSize {
			return nil, fmt.Errorf("query length %d is too big", msgSize)
		}
		msgBuf := pool.GetMsgBuf(msgSize)
		defer pool.ReleaseMsgBuf(msgBuf)
		strBuf := readBufPool.Get()
		defer readBufPool.Release(strBuf)

		strBuf.WriteString(s)
		n, err := base64.RawURLEncoding.Decode(msgBuf, strBuf.Bytes())
		if err != nil {
			return nil, fmt.Errorf("failed to decode query: %w", err)
		}
		b = msgBuf[:n]

	case http.MethodPost:
		buf := readBufPool.Get()
		defer readBufPool.Release(buf)

		_, err = buf.ReadFrom(io.LimitReader(req.Body, dns.MaxMsgSize))
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		b = buf.Bytes()
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}

	q := new(dns.Msg)
	if err := q.Unpack(b); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	return q, nil
}

var readBufPool = pool.NewBytesBufPool(512)

type httpDnsRespWriter struct {
	httpRespWriter http.ResponseWriter
}

func (h *httpDnsRespWriter) Write(m *dns.Msg) (n int, err error) {
	return dnsutils.WriteMsgToUDP(h.httpRespWriter, m)
}
