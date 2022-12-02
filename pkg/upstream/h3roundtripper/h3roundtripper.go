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

package h3roundtripper

import (
	"context"
	"crypto/tls"
	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"go.uber.org/zap"
	"net/http"
	"sync"
	"time"
)

const (
	retryThreshold = time.Millisecond * 50
)

var (
	nopLogger = zap.NewNop()
)

// H3RTHelper is a helper of original http3.RoundTripper.
// This is a workaround of
// https://github.com/lucas-clemente/quic-go/issues/765
type H3RTHelper struct {
	Logger     *zap.Logger
	TLSConfig  *tls.Config
	QUICConfig *quic.Config
	DialFunc   func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error)

	m  sync.Mutex
	rt *http3.RoundTripper
}

func (h *H3RTHelper) logger() *zap.Logger {
	if h.Logger == nil {
		return nopLogger
	}
	return h.Logger
}

func (h *H3RTHelper) getRT() *http3.RoundTripper {
	h.m.Lock()
	defer h.m.Unlock()
	if h.rt == nil {
		h.rt = &http3.RoundTripper{
			Dial:            h.DialFunc,
			TLSClientConfig: h.TLSConfig,
			QuicConfig:      h.QUICConfig,
		}
	}
	return h.rt
}

func (h *H3RTHelper) markAsDead(rt *http3.RoundTripper, err error) {
	h.m.Lock()
	ok := h.rt == rt
	if ok {
		h.rt = nil
	}
	h.m.Unlock()
	if ok {
		_ = rt.Close()
		h.logger().Debug("quic round trip closed", zap.Error(err))
	}
}

func (h *H3RTHelper) RoundTrip(request *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := h.roundTrip(request)
	if err != nil {
		if time.Since(start) < retryThreshold {
			return h.roundTrip(request)
		}
	}
	return resp, err
}

func (h *H3RTHelper) roundTrip(request *http.Request) (*http.Response, error) {
	rt := h.getRT()
	resp, err := rt.RoundTrip(request)
	if err != nil {
		h.markAsDead(rt, err)
	}
	return resp, err
}
