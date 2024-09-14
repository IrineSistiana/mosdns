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
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type HttpHandlerOpts struct {
	// GetSrcIPFromHeader specifies the header that contain client source address.
	// e.g. "X-Forwarded-For".
	GetSrcIPFromHeader string

	// Logger specifies the logger which Handler writes its log to.
	// Default is a nop logger.
	Logger *zap.Logger
}

type HttpHandler struct {
	dnsHandler  Handler
	logger      *zap.Logger
	srcIPHeader string
}

var _ http.Handler = (*HttpHandler)(nil)

func NewHttpHandler(h Handler, opts HttpHandlerOpts) *HttpHandler {
	hh := new(HttpHandler)
	hh.dnsHandler = h
	hh.srcIPHeader = opts.GetSrcIPFromHeader
	hh.logger = opts.Logger
	if hh.logger == nil {
		hh.logger = nopLogger
	}
	return hh
}

func (h *HttpHandler) warnErr(req *http.Request, msg string, err error) {
	h.logger.Warn(msg, zap.String("from", req.RemoteAddr), zap.String("method", req.Method), zap.String("url", req.RequestURI), zap.Error(err))
}

func (h *HttpHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// If server is on unix domain socket, RemoteAddr is not ip:port.
	// Just ignore it.
	// https://github.com/IrineSistiana/mosdns/issues/830
	addrPort, _ := netip.ParseAddrPort(req.RemoteAddr)
	clientAddr := addrPort.Addr()

	// read remote addr from header
	if header := h.srcIPHeader; len(header) != 0 {
		if xff := req.Header.Get(header); len(xff) != 0 {
			addr, err := readClientAddrFromXFF(xff)
			if err != nil {
				h.warnErr(req, "failed to get client ip from header", fmt.Errorf("failed to prase header %s: %s, %s", header, xff, err))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			clientAddr = addr
		}
	}

	// read msg
	q, err := ReadMsgFromReq(req)
	if err != nil {
		h.warnErr(req, "invalid request", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	queryMeta := QueryMeta{
		ClientAddr: clientAddr,
	}
	if u := req.URL; u != nil {
		queryMeta.UrlPath = u.Path
	}
	if tlsStat := req.TLS; tlsStat != nil {
		queryMeta.ServerName = tlsStat.ServerName
	}
	resp := h.dnsHandler.Handle(req.Context(), q, queryMeta, pool.PackBuffer)
	if resp == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer pool.ReleaseBuf(resp)
	w.Header().Set("Content-Type", "application/dns-message")
	if _, err := w.Write(*resp); err != nil {
		h.warnErr(req, "failed to write response", err)
		return
	}
}

func readClientAddrFromXFF(s string) (netip.Addr, error) {
	if i := strings.IndexRune(s, ','); i > 0 {
		return netip.ParseAddr(s[:i])
	}
	return netip.ParseAddr(s)
}

var errInvalidMediaType = errors.New("missing or invalid media type header")

var bufPool = pool.NewBytesBufPool(512)

func ReadMsgFromReq(req *http.Request) (*dns.Msg, error) {
	var b []byte

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

		var err error
		b, err = base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 query: %w", err)
		}

	case http.MethodPost:
		// Check Content-Type header
		if req.Header.Get("Content-Type") != "application/dns-message" {
			return nil, errInvalidMediaType
		}

		buf := bufPool.Get()
		defer bufPool.Release(buf)
		_, err := buf.ReadFrom(io.LimitReader(req.Body, dns.MaxMsgSize))
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		b = buf.Bytes()
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}

	m := new(dns.Msg)
	if err := m.Unpack(b); err != nil {
		return nil, fmt.Errorf("failed to unpack msg [%x], %w", b, err)
	}
	return m, nil
}
