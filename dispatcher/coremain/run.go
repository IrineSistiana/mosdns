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

package coremain

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/load_cache"
	_ "github.com/IrineSistiana/mosdns/v3/dispatcher/plugin"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
)

// Run starts mosdns, it blocks.
func Run(c string) {
	if err := loadConfig(c, 0); err != nil {
		mlog.L().Fatal("failed to load config", zap.Error(err))
	}

	mlog.L().Info("all plugins are successfully loaded")
	load_cache.GetCache().Purge()
	debug.FreeOSMemory()

	//wait for signals
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
	s := <-osSignals
	mlog.L().Info("exiting, bye", zap.Stringer("signal", s))
	os.Exit(0)
}

const (
	maxIncludeDepth = 16
)

var errIncludeDepth = errors.New("max include depth reached")

// Init loads plugins from config
func loadConfig(f string, depth int) error {
	if depth >= maxIncludeDepth {
		return errIncludeDepth
	}
	depth++

	mlog.L().Info("loading config", zap.String("file", f))
	c, err := parseConfig(f)
	if err != nil {
		return fmt.Errorf("failed to parse the config file: %w", err)
	}

	if depth == 1 {
		if err := mlog.ApplyGlobalConfig(&c.Log); err != nil {
			return fmt.Errorf("failed to init logger: %w", err)
		}

		for _, lib := range c.Library {
			if err := openGoPlugin(lib); err != nil {
				return err
			}
		}
	}

	for _, pluginConfig := range c.Plugin {
		if len(pluginConfig.Tag) == 0 || len(pluginConfig.Type) == 0 {
			continue
		}

		mlog.L().Info("loading plugin", zap.String("tag", pluginConfig.Tag))
		p, err := handler.NewPlugin(pluginConfig)
		if err != nil {
			return fmt.Errorf("failed to init plugin %s: %v", pluginConfig.Tag, err)
		}
		if !handler.RegPlugin(p) {
			p.Shutdown()
			return fmt.Errorf("plugin tag %s has been registered", pluginConfig.Tag)
		}
	}

	for _, include := range c.Include {
		if err := loadConfig(include, depth); err != nil {
			return err
		}
	}
	return nil
}
