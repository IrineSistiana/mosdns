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
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net/http"
	"sync"
)

var (
	nopLogger = zap.NewNop()
)

type Handler struct {
	// DNSHandler is required.
	DNSHandler dns_handler.Handler

	// Path specifies the query endpoint. If it is empty, Handler
	// will ignore the request path.
	Path string

	// SrcIPHeader specifies the header that contain client source address.
	// e.g. "X-Forwarded-For".
	SrcIPHeader string

	// Logger specifies the logger which Handler writes its log to.
	Logger *zap.Logger
}

func (h *Handler) logger() *zap.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return nopLogger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// check url path
	if len(h.Path) != 0 && req.URL.Path != h.Path {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// read remote addr
	remoteAddr := req.RemoteAddr
	if len(h.SrcIPHeader) != 0 {
		if ip := req.Header.Get(h.SrcIPHeader); len(ip) != 0 {
			remoteAddr = ip + ":0"
		}
	}

	// read msg
	m, err := ReadMsgFromReq(req)
	if err != nil {
		h.logger().Warn("invalid request", zap.String("from", remoteAddr), zap.String("url", req.RequestURI), zap.String("method", req.Method), zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	h.DNSHandler.ServeDNS(
		req.Context(),
		&dns_handler.Request{Msg: m, From: utils.NewNetAddr(remoteAddr, req.URL.Scheme)},
		&httpDnsRespWriter{httpRespWriter: w},
	)
}

var errInvalidMediaType = errors.New("missing or invalid media type header")

func ReadMsgFromReq(req *http.Request) ([]byte, error) {
	switch req.Method {
	case http.MethodGet:
		// Check accept header
		if req.Header.Get("Accept") != "application/dns-message" {
			return nil, errInvalidMediaType
		}

		s := req.URL.Query().Get("dns")
		if len(s) == 0 {
			return nil, errors.New("no dns parameter")
		}
		msgSize := base64.RawURLEncoding.DecodedLen(len(s))
		if msgSize > dns.MaxMsgSize {
			return nil, fmt.Errorf("msg length %d is too big", msgSize)
		}

		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 query: %w", err)
		}
		return b, nil

	case http.MethodPost:
		// Check Content-Type header
		if req.Header.Get("Content-Type") != "application/dns-message" {
			return nil, errInvalidMediaType
		}

		buf := bytes.NewBuffer(make([]byte, 64))
		_, err := buf.ReadFrom(io.LimitReader(req.Body, dns.MaxMsgSize))
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}
}

type httpDnsRespWriter struct {
	setMediaTypeOnce sync.Once
	httpRespWriter   http.ResponseWriter
}

func (h *httpDnsRespWriter) Write(m []byte) (n int, err error) {
	h.setMediaTypeOnce.Do(func() {
		h.httpRespWriter.Header().Set("Content-Type", "application/dns-message")
	})
	return h.httpRespWriter.Write(m)
}
