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

package metrics_collector

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

const PluginType = "metrics_collector"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.RecursiveExecutable = (*Collector)(nil)

type Collector struct {
	queryTotal      prometheus.Counter
	errTotal        prometheus.Counter
	thread          prometheus.Gauge
	responseLatency prometheus.Histogram
}

// NewCollector inits a new Collector with given name to r.
// name must be unique in the r.
func NewCollector(r prometheus.Registerer, name string) (*Collector, error) {
	if len(name) == 0 {
		return nil, errors.New("collector must has a name")
	}

	lb := map[string]string{"name": name}
	var c = &Collector{
		queryTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "query_total",
			Help:        "The total number of queries pass through",
			ConstLabels: lb,
		}),
		errTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "err_total",
			Help:        "The total number of queries failed",
			ConstLabels: lb,
		}),
		thread: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "thread",
			Help:        "The number of threads that are currently being processed",
			ConstLabels: lb,
		}),
		responseLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        "response_latency_millisecond",
			Help:        "The response latency in millisecond",
			Buckets:     []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000},
			ConstLabels: lb,
		}),
	}
	for _, collector := range [...]prometheus.Collector{c.queryTotal, c.errTotal, c.thread, c.responseLatency} {
		if err := r.Register(collector); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Collector) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	c.thread.Inc()
	defer c.thread.Dec()

	c.queryTotal.Inc()
	start := time.Now()
	err := next.ExecNext(ctx, qCtx)
	if err != nil {
		c.errTotal.Inc()
	}
	if qCtx.R() != nil {
		c.responseLatency.Observe(float64(time.Since(start).Milliseconds()))
	}
	return err
}

// QuickSetup format: metrics_name
func QuickSetup(bp sequence.BQ, s string) (any, error) {
	r := prometheus.WrapRegistererWithPrefix(PluginType+"_", bp.M().GetMetricsReg())
	return NewCollector(r, s)
}
