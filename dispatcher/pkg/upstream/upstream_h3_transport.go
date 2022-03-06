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

package upstream

import (
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

// h3rt is a helper of original http3.RoundTripper.
// This is a workaround of
// https://github.com/lucas-clemente/quic-go/issues/765
type h3rt struct {
	logger     *zap.Logger
	tlsConfig  *tls.Config
	quicConfig *quic.Config
	dialFunc   func(network, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlySession, error)

	m  sync.Mutex
	rt *http3.RoundTripper
}

func (h *h3rt) getLogger() *zap.Logger {
	if h.logger == nil {
		return nopLogger
	}
	return h.logger
}

func (h *h3rt) getRT() *http3.RoundTripper {
	h.m.Lock()
	defer h.m.Unlock()
	if h.rt == nil {
		h.rt = &http3.RoundTripper{
			TLSClientConfig:        h.tlsConfig,
			QuicConfig:             h.quicConfig,
			Dial:                   h.dialFunc,
			MaxResponseHeaderBytes: 512,
		}
	}
	return h.rt
}

func (h *h3rt) markAsDead(rt *http3.RoundTripper) {
	h.m.Lock()
	defer h.m.Unlock()
	if h.rt == rt {
		h.rt = nil
	}
}

func (h *h3rt) RoundTrip(request *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := h.roundTrip(request)
	if err != nil {
		if time.Since(start) < retryThreshold {
			return h.roundTrip(request)
		}
	}
	return resp, err
}

func (h *h3rt) roundTrip(request *http.Request) (*http.Response, error) {
	rt := h.getRT()
	resp, err := rt.RoundTrip(request)
	if err != nil {
		h.markAsDead(rt)
		rt.Close()
		h.getLogger().Debug("quic round trip closed", zap.Error(err))
	}
	return resp, err
}
