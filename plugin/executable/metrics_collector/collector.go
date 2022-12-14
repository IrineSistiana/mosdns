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

func NewCollector(bq sequence.BQ, nameSpace string) *Collector {
	var c = &Collector{
		queryTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "query_total",
			Help: "The total number of queries pass through this collector",
		}),
		errTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "err_total",
			Help: "The total number of queries failed after this collector",
		}),
		thread: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "thread",
			Help: "The number of threads currently through this collector",
		}),
		responseLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "response_latency_millisecond",
			Help:    "The response latency in millisecond",
			Buckets: []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000},
		}),
	}

	var register func(...prometheus.Collector)
	if len(nameSpace) > 0 {
		register = prometheus.WrapRegistererWithPrefix(nameSpace, bq.M().GetMetricsReg()).MustRegister
	} else {
		register = bq.M().GetMetricsReg().MustRegister
	}
	register(c.queryTotal, c.errTotal, c.thread, c.responseLatency)
	return c
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
	return NewCollector(bp, s), nil
}
