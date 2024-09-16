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
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap/zapcore"
)

type upstreamWrapper struct {
	idx             int
	u               upstream.Upstream
	cfg             UpstreamConfig
	queryTotal      prometheus.Counter
	errTotal        prometheus.Counter
	thread          prometheus.Gauge
	responseLatency prometheus.Histogram

	connOpened prometheus.Counter
	connClosed prometheus.Counter
}

func (uw *upstreamWrapper) OnEvent(typ upstream.Event) {
	switch typ {
	case upstream.EventConnOpen:
		uw.connOpened.Inc()
	case upstream.EventConnClose:
		uw.connClosed.Inc()
	}
}

// newWrapper inits all metrics.
// Note: upstreamWrapper.u still needs to be set.
func newWrapper(idx int, cfg UpstreamConfig, pluginTag string) *upstreamWrapper {
	lb := map[string]string{"upstream": cfg.Tag, "tag": pluginTag}
	return &upstreamWrapper{
		cfg: cfg,
		queryTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "query_total",
			Help:        "The total number of queries processed by this upstream",
			ConstLabels: lb,
		}),
		errTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "err_total",
			Help:        "The total number of queries failed",
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

		connOpened: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "conn_opened_total",
			Help:        "The total number of connections that are opened",
			ConstLabels: lb,
		}),
		connClosed: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "conn_closed_total",
			Help:        "The total number of connections that are closed",
			ConstLabels: lb,
		}),
	}
}

func (uw *upstreamWrapper) registerMetricsTo(r prometheus.Registerer) error {
	for _, collector := range [...]prometheus.Collector{
		uw.queryTotal,
		uw.errTotal,
		uw.thread,
		uw.responseLatency,
		uw.connOpened,
		uw.connClosed,
	} {
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

func (uw *upstreamWrapper) ExchangeContext(ctx context.Context, m []byte) (*[]byte, error) {
	uw.queryTotal.Inc()

	start := time.Now()
	uw.thread.Inc()
	r, err := uw.u.ExchangeContext(ctx, m)
	uw.thread.Dec()

	if err != nil {
		uw.errTotal.Inc()
	} else {
		uw.responseLatency.Observe(float64(time.Since(start).Milliseconds()))
	}
	return r, err
}

func (uw *upstreamWrapper) Close() error {
	return uw.u.Close()
}

type queryInfo dns.Msg

func (q *queryInfo) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	if len(q.Question) != 1 {
		encoder.AddBool("odd_question", true)
	} else {
		question := q.Question[0]
		encoder.AddString("qname", question.Name)
		encoder.AddUint16("qtype", question.Qtype)
		encoder.AddUint16("qclass", question.Qclass)
	}
	return nil
}

func copyPayload(b *[]byte) *[]byte {
	bc := pool.GetBuf(len(*b))
	copy(*bc, *b)
	return bc
}
