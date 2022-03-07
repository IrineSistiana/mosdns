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
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"plugin"
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
		// init logger
		level, ok := parseLogLevel(c.Log.Level)
		if !ok {
			return fmt.Errorf("invalid log level [%s]", c.Log.Level)
		}
		mlog.Level().SetLevel(level)

		if len(c.Log.InfoFile) == 0 {
			c.Log.InfoFile = c.Log.File
		}
		if len(c.Log.ErrFile) == 0 {
			c.Log.ErrFile = c.Log.File
		}

		if lf := c.Log.File; len(lf) > 0 {
			mlog.L().Info("opening log file", zap.String("file", lf))
			f, err := os.OpenFile(lf, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
			if err != nil {
				return fmt.Errorf("open log file: %w", err)
			}
			mlog.L().Info("redirecting log to file, end of console log", zap.String("file", c.Log.File))
			fLocked := zapcore.Lock(f)
			mlog.InfoWriter().Replace(fLocked)
			mlog.ErrWriter().Replace(fLocked)
		} else {
			if lf := c.Log.InfoFile; len(lf) > 0 {
				mlog.L().Info("opening info log file", zap.String("file", lf))
				f, err := os.OpenFile(lf, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					return fmt.Errorf("open info log file: %w", err)
				}
				mlog.L().Info("redirecting info log to file, end of console log", zap.String("file", c.Log.File))
				mlog.InfoWriter().Replace(zapcore.Lock(f))
			}
			if lf := c.Log.ErrFile; len(lf) > 0 {
				mlog.L().Info("opening err log file", zap.String("file", lf))
				f, err := os.OpenFile(lf, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					return fmt.Errorf("open err log file: %w", err)
				}
				mlog.L().Info("redirecting err log to file, end of console log", zap.String("file", c.Log.File))
				mlog.ErrWriter().Replace(zapcore.Lock(f))
			}
		}

		for _, lib := range c.Library {
			mlog.L().Info("loading library", zap.String("library", lib))
			_, err := plugin.Open(lib)
			if err != nil {
				return fmt.Errorf("failed to open library: %w", err)
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

func parseLogLevel(s string) (zapcore.Level, bool) {
	switch s {
	case "debug":
		return zap.DebugLevel, true
	case "", "info":
		return zap.InfoLevel, true
	case "warn":
		return zap.WarnLevel, true
	case "error":
		return zap.ErrorLevel, true
	default:
		return 0, false
	}
}
