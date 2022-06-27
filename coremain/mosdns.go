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
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/mlog"
	"github.com/IrineSistiana/mosdns/v4/pkg/data_provider"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/metrics"
	"github.com/IrineSistiana/mosdns/v4/pkg/notifier"
	"go.uber.org/zap"
	"net/http"
	"net/http/pprof"
	"runtime"
	"runtime/debug"
	"time"
)

type Mosdns struct {
	logger *zap.Logger

	// Data
	dataManager *data_provider.DataManager

	// Plugins
	execs    map[string]executable_seq.Executable
	matchers map[string]executable_seq.Matcher

	httpAPIMux    *http.ServeMux
	httpAPIServer *http.Server

	rootMetricsReg    *metrics.Registry
	pluginsMetricsReg *metrics.Registry
	serversMetricsReg *metrics.Registry

	sc *notifier.SafeClose
}

func RunMosdns(cfg *Config) error {
	lg, err := mlog.NewLogger(&cfg.Log)
	if err != nil {
		return err
	}

	m := &Mosdns{
		logger:      lg,
		dataManager: data_provider.NewDataManager(),
		execs:       make(map[string]executable_seq.Executable),
		matchers:    make(map[string]executable_seq.Matcher),
		httpAPIMux:  http.NewServeMux(),
		sc:          notifier.NewSafeClose(),
	}
	m.rootMetricsReg = metrics.NewRegistry()
	m.pluginsMetricsReg = metrics.NewRegistry()
	m.serversMetricsReg = metrics.NewRegistry()
	m.rootMetricsReg.Set("plugins", m.pluginsMetricsReg)
	m.rootMetricsReg.Set("servers", m.serversMetricsReg)

	m.httpAPIMux.HandleFunc("/metrics/", metrics.HandleFunc(m.rootMetricsReg, m.logger))
	m.httpAPIMux.HandleFunc("/debug/pprof/", pprof.Index)
	m.httpAPIMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	m.httpAPIMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.httpAPIMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.httpAPIMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Init data manager
	dupTag := make(map[string]struct{})
	for _, dpc := range cfg.DataProviders {
		if len(dpc.Tag) == 0 {
			continue
		}
		if _, ok := dupTag[dpc.Tag]; ok {
			return fmt.Errorf("duplicated provider tag %s", dpc.Tag)
		}
		dupTag[dpc.Tag] = struct{}{}

		dp, err := data_provider.NewDataProvider(lg, &dpc)
		if err != nil {
			return fmt.Errorf("failed to init data provider %s, %w", dpc.Tag, err)
		}
		m.dataManager.AddDataProvider(dpc.Tag, dp)
	}

	// Init preset plugins
	for tag, f := range LoadNewPersetPluginFuncs() {
		p, err := f(NewBP(tag, "preset", m.logger, m))
		if err != nil {
			return fmt.Errorf("failed to init preset plugin %s, %w", tag, err)
		}
		m.addPlugin(p)
	}

	// Init plugins
	dupTag = make(map[string]struct{})
	for i, pc := range cfg.Plugins {
		if len(pc.Type) == 0 || len(pc.Tag) == 0 {
			continue
		}
		if _, dup := dupTag[pc.Tag]; dup {
			return fmt.Errorf("duplicated plugin tag %s", pc.Tag)
		}
		dupTag[pc.Tag] = struct{}{}

		m.logger.Info("loading plugin", zap.String("tag", pc.Tag), zap.String("type", pc.Type))
		p, err := NewPlugin(&pc, m.logger, m)
		if err != nil {
			return fmt.Errorf("failed to init plugin #%d, %w", i, err)
		}

		m.addPlugin(p)
		// Also add it to api mux if plugin implements http.Handler.
		if h, ok := p.(http.Handler); ok {
			m.httpAPIMux.Handle(fmt.Sprintf("/plugins/%s/", p.Tag()), h)
		}
	}

	if len(cfg.Servers) == 0 {
		return errors.New("no server is configured")
	}
	for i, sc := range cfg.Servers {
		if err := m.startServers(&sc); err != nil {
			return fmt.Errorf("failed to start server #%d, %w", i, err)
		}
	}

	// Start http api server
	if httpAddr := cfg.API.HTTP; len(httpAddr) > 0 {
		httpServer := &http.Server{
			Addr:    httpAddr,
			Handler: m.httpAPIMux,
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
				httpServer.Close()
			}
		})
	}

	time.AfterFunc(time.Second*1, func() {
		runtime.GC()
		debug.FreeOSMemory()
	})
	<-m.sc.ReceiveCloseSignal()
	m.sc.Done()
	m.sc.CloseWait()
	return m.sc.Err()
}

func (m *Mosdns) addPlugin(p Plugin) {
	t := p.Tag()
	if p, ok := p.(ExecutablePlugin); ok {
		m.execs[t] = p
	}
	if p, ok := p.(MatcherPlugin); ok {
		m.matchers[p.Tag()] = p
	}
}

func (m *Mosdns) GetDataManager() *data_provider.DataManager {
	return m.dataManager
}

func (m *Mosdns) GetSafeClose() *notifier.SafeClose {
	return m.sc
}

func (m *Mosdns) GetExecutables() map[string]executable_seq.Executable {
	return m.execs
}

func (m *Mosdns) GetPluginMetricsReg() *metrics.Registry {
	return m.pluginsMetricsReg
}

func (m *Mosdns) GetServerMetricsReg() *metrics.Registry {
	return m.serversMetricsReg
}

func (m *Mosdns) GetMatchers() map[string]executable_seq.Matcher {
	return m.matchers
}

// GetHTTPAPIMux returns the api http.ServeMux.
// The pattern "/plugins/plugin_tag/" has been registered if
// Plugin implements http.Handler interface.
// Plugin caller should register path that has "/plugins/plugin_tag/"
// prefix only.
func (m *Mosdns) GetHTTPAPIMux() *http.ServeMux {
	return m.httpAPIMux
}
