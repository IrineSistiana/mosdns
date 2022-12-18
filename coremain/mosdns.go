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

package coremain

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/safe_close"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/http/pprof"
)

type Mosdns struct {
	logger *zap.Logger // non-nil logger.

	// Plugins
	plugins map[string]any

	httpMux    *chi.Mux
	metricsReg *prometheus.Registry
	sc         *safe_close.SafeClose
}

// NewMosdns initializes a mosdns instance and its plugins.
func NewMosdns(cfg *Config) (*Mosdns, error) {
	// Init logger.
	lg, err := mlog.NewLogger(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("failed to init logger: %w", err)
	}

	m := &Mosdns{
		logger:     lg,
		plugins:    make(map[string]any),
		httpMux:    chi.NewRouter(),
		metricsReg: newMetricsReg(),
		sc:         safe_close.NewSafeClose(),
	}
	// This must be called after m.httpMux and m.metricsReg been set.
	m.initHttpMux()

	// Start http api server
	if httpAddr := cfg.API.HTTP; len(httpAddr) > 0 {
		httpServer := &http.Server{
			Addr:    httpAddr,
			Handler: m.httpMux,
		}
		m.sc.Attach(func(done func(), closeSignal <-chan struct{}) {
			defer done()
			errChan := make(chan error, 1)
			go func() {
				m.logger.Info("starting api http server", zap.String("addr", httpAddr))
				errChan <- httpServer.ListenAndServe()
			}()
			select {
			case err := <-errChan:
				m.sc.SendCloseSignal(err)
			case <-closeSignal:
				_ = httpServer.Close()
			}
		})
	}

	// Load plugins.

	// Close all plugins on signal.
	// From here, call m.sc.SendCloseSignal() if any plugin failed to load.
	m.sc.Attach(func(done func(), closeSignal <-chan struct{}) {
		go func() {
			defer done()
			<-closeSignal
			m.logger.Info("starting shutdown sequences")
			for tag, p := range m.plugins {
				if closer, _ := p.(io.Closer); closer != nil {
					m.logger.Info("closing plugin", zap.String("tag", tag))
					_ = closer.Close()
				}
			}
			m.logger.Info("all plugins were closed")
		}()
	})

	// Preset plugins
	if err := m.loadPresetPlugins(); err != nil {
		m.sc.SendCloseSignal(err)
		return nil, err
	}
	// Plugins from config.
	if err := m.loadPluginsFromCfg(cfg, 0); err != nil {
		m.sc.SendCloseSignal(err)
		return nil, err
	}
	m.logger.Info("all plugins are loaded")

	return m, nil
}

// NewTestMosdnsWithPlugins returns a mosdns instance for testing.
func NewTestMosdnsWithPlugins(p map[string]any) *Mosdns {
	return &Mosdns{
		logger:     mlog.Nop(),
		httpMux:    chi.NewRouter(),
		plugins:    p,
		metricsReg: newMetricsReg(),
		sc:         safe_close.NewSafeClose(),
	}
}

func (m *Mosdns) GetSafeClose() *safe_close.SafeClose {
	return m.sc
}

// CloseWithErr is a shortcut for m.sc.SendCloseSignal
func (m *Mosdns) CloseWithErr(err error) {
	m.sc.SendCloseSignal(err)
}

// Logger returns a non-nil logger.
func (m *Mosdns) Logger() *zap.Logger {
	return m.logger
}

// GetPlugin returns a plugin.
func (m *Mosdns) GetPlugin(tag string) any {
	return m.plugins[tag]
}

// GetMetricsReg returns a prometheus.Registerer with a prefix of "mosdns_"
func (m *Mosdns) GetMetricsReg() prometheus.Registerer {
	return prometheus.WrapRegistererWithPrefix("mosdns_", m.metricsReg)
}

func (m *Mosdns) GetAPIRouter() *chi.Mux {
	return m.httpMux
}

func (m *Mosdns) RegPluginAPI(tag string, mux *chi.Mux) {
	m.httpMux.Mount("/plugins/"+tag, mux)
}

func newMetricsReg() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewGoCollector())
	return reg
}

// initHttpMux initializes api entries. It MUST be called after m.metricsReg being initialized.
func (m *Mosdns) initHttpMux() {
	// Register metrics.
	m.httpMux.Method(http.MethodGet, "/metrics", promhttp.HandlerFor(m.metricsReg, promhttp.HandlerOpts{}))

	// Register pprof.
	m.httpMux.Route("/debug/pprof", func(r chi.Router) {
		r.Get("/*", pprof.Index)
		r.Get("/cmdline", pprof.Cmdline)
		r.Get("/profile", pprof.Profile)
		r.Get("/symbol", pprof.Symbol)
		r.Get("/trace", pprof.Trace)
	})

	// A helper page for invalid request.
	invalidApiReqHelper := func(w http.ResponseWriter, req *http.Request) {
		b := new(bytes.Buffer)
		_, _ = fmt.Fprintf(b, "Invalid request %s %s\n\n", req.Method, req.RequestURI)
		b.WriteString("Available api urls:\n")
		_ = chi.Walk(m.httpMux, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			b.WriteString(method)
			b.WriteByte(' ')
			b.WriteString(route)
			b.WriteByte('\n')
			return nil
		})
		_, _ = w.Write(b.Bytes())
	}
	m.httpMux.NotFound(invalidApiReqHelper)
	m.httpMux.MethodNotAllowed(invalidApiReqHelper)
}

func (m *Mosdns) loadPresetPlugins() error {
	for tag, f := range LoadNewPersetPluginFuncs() {
		p, err := f(NewBP(tag, m))
		if err != nil {
			return fmt.Errorf("failed to init preset plugin %s, %w", tag, err)
		}
		m.plugins[tag] = p
	}
	return nil
}

// loadPluginsFromCfg loads plugins from this config. It follows include first.
func (m *Mosdns) loadPluginsFromCfg(cfg *Config, includeDepth int) error {
	const maxIncludeDepth = 8
	if includeDepth > maxIncludeDepth {
		return errors.New("maximum include depth reached")
	}
	includeDepth++

	// Follow include first.
	for _, s := range cfg.Include {
		subCfg, path, err := loadConfig(s)
		if err != nil {
			return fmt.Errorf("failed to read config from %s, %w", s, err)
		}
		m.logger.Info("load config", zap.String("file", path))
		if err := m.loadPluginsFromCfg(subCfg, includeDepth); err != nil {
			return fmt.Errorf("failed to load config from %s, %w", s, err)
		}
	}

	for i, pc := range cfg.Plugins {
		if err := m.newPlugin(pc); err != nil {
			return fmt.Errorf("failed to init plugin #%d %s, %w", i, pc.Tag, err)
		}
	}
	return nil
}
