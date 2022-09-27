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

package http_handler

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/ip_observer"
	"github.com/IrineSistiana/mosdns/v4/pkg/pool"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v4/pkg/server/dns_handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/netip"
	"strings"
)

var (
	nopLogger = zap.NewNop()
)

type HandlerOpts struct {
	// DNSHandler is required.
	DNSHandler dns_handler.Handler

	// Path specifies the query endpoint. If it is empty, Handler
	// will ignore the request path.
	Path string

	// SrcIPHeader specifies the header that contain client source address.
	// e.g. "X-Forwarded-For".
	SrcIPHeader string

	// Logger specifies the logger which Handler writes its log to.
	// Default is a nop logger.
	Logger *zap.Logger

	// Optional.
	BadIPObserver ip_observer.IPObserver
}

func (opts *HandlerOpts) Init() error {
	if opts.DNSHandler == nil {
		return errors.New("nil dns handler")
	}
	if opts.Logger == nil {
		opts.Logger = nopLogger
	}
	if opts.BadIPObserver == nil {
		opts.BadIPObserver = ip_observer.NewNopObserver()
	}
	return nil
}

type Handler struct {
	opts HandlerOpts
}

func NewHandler(opts HandlerOpts) (*Handler, error) {
	if err := opts.Init(); err != nil {
		return nil, err
	}
	return &Handler{opts: opts}, nil
}

func (h *Handler) warnErr(req *http.Request, msg string, err error) {
	h.opts.Logger.Warn(msg, zap.String("from", req.RemoteAddr), zap.String("method", req.Method), zap.String("url", req.RequestURI), zap.Error(err))
}

// If addr is invalid, badAddr is a noop.
func (h *Handler) possibleBadAddr(addr netip.Addr) {
	if addr.IsValid() {
		h.opts.BadIPObserver.Observe(addr)
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	addrPort, err := netip.ParseAddrPort(req.RemoteAddr)
	if err != nil {
		h.opts.Logger.Error("failed to parse request remote addr", zap.String("addr", req.RemoteAddr), zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	clientAddr := addrPort.Addr()

	// read remote addr from header
	if header := h.opts.SrcIPHeader; len(header) != 0 {
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

	// check url path
	if len(h.opts.Path) != 0 && req.URL.Path != h.opts.Path {
		h.possibleBadAddr(clientAddr)
		w.WriteHeader(http.StatusNotFound)
		h.warnErr(req, "invalid request", fmt.Errorf("invalid request path %s", req.URL.Path))
		return
	}

	// read msg
	q, err := ReadMsgFromReq(req)
	if err != nil {
		h.possibleBadAddr(clientAddr)
		h.warnErr(req, "invalid request", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	r, err := h.opts.DNSHandler.ServeDNS(req.Context(), q, &query_context.RequestMeta{ClientAddr: clientAddr})
	if err != nil {
		h.possibleBadAddr(clientAddr)
		panic(err.Error()) // Force http server to close connection.
	}

	b, buf, err := pool.PackBuffer(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.warnErr(req, "failed to unpack handler's response", err)
		return
	}
	defer buf.Release()

	w.Header().Set("Content-Type", "application/dns-message")
	if _, err := w.Write(b); err != nil {
		h.possibleBadAddr(clientAddr)
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
