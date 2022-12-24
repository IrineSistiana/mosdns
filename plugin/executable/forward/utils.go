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

package fastforward

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

type upstreamWrapper struct {
	u               upstream.Upstream
	cfg             UpstreamConfig
	queryTotal      prometheus.Counter
	errTotal        prometheus.Counter
	thread          prometheus.Gauge
	responseLatency prometheus.Histogram
}

func wrapUpstream(u upstream.Upstream, cfg UpstreamConfig, pluginTag string) *upstreamWrapper {
	lb := map[string]string{"upstream": cfg.Tag, "tag": pluginTag}
	return &upstreamWrapper{
		u:   u,
		cfg: cfg,
		queryTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "query_total",
			Help:        "The total number of queries processed by this upstream",
			ConstLabels: lb,
		}),
		errTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "err_total",
			Help:        "The total number of queries failed, including SERVFAIL",
			ConstLabels: lb,
		}),
		thread: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "thread",
			Help:        "The number of threads (queries) that are currently being processed",
			ConstLabels: lb,
		}),
		responseLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        "response_latency_millisecond",
			Help:        "The response latency in millisecond",
			Buckets:     []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000},
			ConstLabels: lb,
		}),
	}
}

func (uw *upstreamWrapper) registerMetricsTo(r prometheus.Registerer) error {
	for _, collector := range [...]prometheus.Collector{uw.queryTotal, uw.errTotal, uw.thread, uw.responseLatency} {
		if err := r.Register(collector); err != nil {
			return err
		}
	}
	return nil
}

// name returns upstream tag if it was set in the config.
// Otherwise, it returns upstream address.
func (uw *upstreamWrapper) name() string {
	if t := uw.cfg.Tag; len(t) > 0 {
		return uw.cfg.Tag
	}
	return uw.cfg.Addr
}

func (uw *upstreamWrapper) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	uw.queryTotal.Inc()

	start := time.Now()
	uw.thread.Inc()
	r, err := uw.u.ExchangeContext(ctx, m)
	uw.thread.Dec()

	if err != nil || (r != nil && r.Rcode == dns.RcodeServerFailure) {
		uw.errTotal.Inc()
	} else {
		uw.responseLatency.Observe(float64(time.Since(start).Milliseconds()))
	}
	return r, err
}

func (uw *upstreamWrapper) Close() error {
	return uw.u.Close()
}
