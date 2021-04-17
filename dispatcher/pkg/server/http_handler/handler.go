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

package http_handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/pool"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net/http"
	"time"
)

const (
	defaultTimeout = 5
)

type Handler struct {
	dnsHandler        dns_handler.Handler
	path              string
	clientSrcIPHeader string
	timeout           time.Duration

	logger *zap.Logger
}

func NewHandler(dnsHandler dns_handler.Handler, options ...Option) *Handler {
	h := new(Handler)
	h.dnsHandler = dnsHandler
	for _, op := range options {
		op(h)
	}

	if h.logger == nil {
		h.logger = zap.NewNop()
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// check url path
	if len(h.path) != 0 && req.URL.Path != h.path {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// read remote addr
	remoteAddr := req.RemoteAddr
	if len(h.clientSrcIPHeader) != 0 {
		if ip := req.Header.Get(h.clientSrcIPHeader); len(ip) != 0 {
			remoteAddr = ip + ":0"
		}
	}

	// read msg
	q, err := ReadMsgFromReq(req)
	if err != nil {
		h.logger.Warn("invalid request", zap.String("from", remoteAddr), zap.String("url", req.RequestURI), zap.String("method", req.Method), zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	qCtx := handler.NewContext(q, utils.NewNetAddr(remoteAddr, req.URL.Scheme))
	qCtx.SetTCPClient(true)
	ctx, cancel := context.WithTimeout(req.Context(), h.getTimeout())
	defer cancel()
	h.dnsHandler.ServeDNS(ctx, qCtx, &httpDnsRespWriter{httpRespWriter: w})
}

func (h *Handler) getTimeout() time.Duration {
	if t := h.timeout; t > 0 {
		return t
	}
	return defaultTimeout
}

func ReadMsgFromReq(req *http.Request) (*dns.Msg, error) {
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
		msgBuf := pool.GetBuf(msgSize)
		defer pool.ReleaseBuf(msgBuf)
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
