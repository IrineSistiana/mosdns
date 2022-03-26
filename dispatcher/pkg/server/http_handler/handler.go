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
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/pool"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server/dns_handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"strings"
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

func (h *Handler) warnErr(req *http.Request, msg string, err error) {
	h.logger().Warn(msg, zap.String("from", req.RemoteAddr), zap.String("method", req.Method), zap.String("url", req.RequestURI), zap.Error(err))
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// check url path
	if len(h.Path) != 0 && req.URL.Path != h.Path {
		w.WriteHeader(http.StatusNotFound)
		h.warnErr(req, "invalid request", fmt.Errorf("invalid request path %s", h.Path))
		return
	}

	// read msg
	m, err := ReadMsgFromReq(req)
	if err != nil {
		h.warnErr(req, "invalid request", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// read remote addr
	var clientIP net.IP
	if header := h.SrcIPHeader; len(header) != 0 {
		if xff := req.Header.Get(header); len(xff) != 0 {
			clientIP = readClientIPFromXFF(xff)
			if clientIP == nil {
				h.warnErr(req, "failed to get client ip", fmt.Errorf("failed to prase header %s: %s", header, xff))
			}
		}
	}

	// If no ip read from the ip header, use the remote address from net/http.
	if clientIP == nil {
		ip, _, _ := net.SplitHostPort(req.RemoteAddr)
		if len(ip) > 0 {
			clientIP = net.ParseIP(ip)
		}
		if clientIP == nil {
			h.warnErr(req, "failed to get client ip", fmt.Errorf("failed to prase request remote addr %s", req.RemoteAddr))
		}
	}

	if err := h.DNSHandler.ServeDNS(
		req.Context(),
		m,
		&httpDnsRespWriter{httpRespWriter: w},
		&handler.RequestMeta{ClientIP: clientIP},
	); err != nil {
		h.warnErr(req, "handler err", err)
		panic(err) // panic can force http server to close the downstream connection.
	}
}

func readClientIPFromXFF(s string) net.IP {
	if i := strings.IndexRune(s, ','); i > 0 {
		return net.ParseIP(s[:i])
	}
	return net.ParseIP(s)
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

type httpDnsRespWriter struct {
	httpRespWriter http.ResponseWriter
}

func (h *httpDnsRespWriter) Write(m *dns.Msg) error {
	b, buf, err := pool.PackBuffer(m)
	if err != nil {
		h.httpRespWriter.WriteHeader(http.StatusInternalServerError)
		return err
	}
	defer buf.Release()

	h.httpRespWriter.Header().Set("Content-Type", "application/dns-message")
	_, err = h.httpRespWriter.Write(b)
	return err
}
