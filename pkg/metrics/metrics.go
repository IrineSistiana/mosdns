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

package metrics

import (
	"encoding/json"
	"github.com/rcrowley/go-metrics"
	"go.uber.org/zap"
	"net/http"
	"sync"
)

type Var interface {
	// Publish should return a build-in type or a map[string]interface{}
	// of build-in types.
	Publish() interface{}
}

type Registry struct {
	l sync.RWMutex
	m map[string]Var
}

func NewRegistry() *Registry {
	return &Registry{
		m: make(map[string]Var),
	}
}

func (r *Registry) Get(name string) Var {
	r.l.RLock()
	defer r.l.RUnlock()
	return r.m[name]
}

func (r *Registry) GetOrSet(name string, f func() Var) Var {
	r.l.Lock()
	defer r.l.Unlock()
	v, ok := r.m[name]
	if !ok {
		v = f()
		r.m[name] = v
	}
	return v
}

func (r *Registry) Set(name string, v Var) {
	r.l.Lock()
	defer r.l.Unlock()
	r.m[name] = v
}

func (r *Registry) Publish() interface{} {
	m := make(map[string]interface{})
	r.l.RLock()
	defer r.l.RUnlock()
	for name, v := range r.m {
		m[name] = v.Publish()
	}
	return m
}

type Histogram struct {
	metrics.Histogram
}

func NewHistogram(reservoirSize int) *Histogram {
	return &Histogram{Histogram: metrics.NewHistogram(metrics.NewUniformSample(reservoirSize))}
}

func (h *Histogram) Publish() interface{} {
	m := make(map[string]interface{})
	ps := h.Percentiles([]float64{0, 0.25, 0.5, 0.75, 1})
	m["min"] = ps[0]
	m["p25"] = ps[1]
	m["p50"] = ps[2]
	m["p75"] = ps[3]
	m["max"] = ps[4]
	m["avg"] = int64(h.Mean())
	return m
}

type Counter struct {
	metrics.Counter
}

func NewCounter() *Counter {
	return &Counter{Counter: metrics.NewCounter()}
}

func (c *Counter) Publish() interface{} {
	return c.Count()
}

type Gauge struct {
	metrics.Gauge
}

func NewGauge() *Gauge {
	return &Gauge{Gauge: metrics.NewGauge()}
}

type GaugeFunc struct {
	l sync.Mutex
	f func() int64
}

func NewGaugeFunc(f func() int64) *GaugeFunc {
	return &GaugeFunc{f: f}
}

func (gf *GaugeFunc) Publish() interface{} {
	gf.l.Lock()
	defer gf.l.Unlock()
	return gf.f()
}

func (g *Gauge) Publish() interface{} {
	return g.Value()
}

func HandleFunc(r Var, lg *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		b, err := json.MarshalIndent(r.Publish(), "", "  ")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			lg.Error("failed to encode json", zap.Error(err))
			return
		}
		if _, err := w.Write(b); err != nil {
			lg.Error("http write err", zap.Error(err))
		}
	}
}
