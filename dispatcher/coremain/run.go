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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
)

// Run starts mosdns, it blocks.
func Run(c string) {
	err := loadConfig(c, 0)
	if err != nil {
		mlog.L().Fatal("loading config", zap.Error(err))
	}

	mlog.L().Info("all plugins are successfully loaded")
	//wait for signals
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
	s := <-osSignals
	mlog.L().Info("exiting, bye", zap.Stringer("signal", s))
	os.Exit(0)
}

const (
	maxIncludeDepth = 10
)

// Init loads plugins from config
func loadConfig(f string, depth int) error {
	if depth >= maxIncludeDepth {
		return errors.New("max include depth reached")
	}
	depth++

	mlog.L().Info("loading config", zap.String("file", f))
	c, err := parseConfig(f)
	if err != nil {
		return fmt.Errorf("failed to parse config from file %s: %w", f, err)
	}

	if depth == 1 { // init logger
		level, err := parseLogLevel(c.Log.Level)
		if err != nil {
			return err
		}
		mlog.Level().SetLevel(level)
		if len(c.Log.File) != 0 {
			mlog.L().Info("opening log file", zap.String("file", c.Log.File))
			f, err := os.OpenFile(c.Log.File, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
			if err != nil {
				return fmt.Errorf("can not open log file %s: %w", c.Log.File, err)
			}
			mlog.L().Info("redirecting log to file, end of console log", zap.String("file", c.Log.File))
			mlog.Writer().Replace(f)
		}
	}

	for i, pluginConfig := range c.Plugin {
		if len(pluginConfig.Tag) == 0 || len(pluginConfig.Type) == 0 {
			continue
		}
		mlog.L().Info("loading plugin", zap.String("tag", pluginConfig.Tag))
		if err := handler.InitAndRegPlugin(pluginConfig, true); err != nil {
			return fmt.Errorf("failed to register plugin #%d %s: %w", i, pluginConfig.Tag, err)
		}
	}

	for _, include := range c.Include {
		if len(include) == 0 {
			continue
		}
		err := loadConfig(include, depth)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseLogLevel(s string) (zapcore.Level, error) {
	switch s {
	case "debug":
		return zap.DebugLevel, nil
	case "", "info":
		return zap.InfoLevel, nil
	case "warn":
		return zap.WarnLevel, nil
	case "error":
		return zap.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("invalid log level [%s]", s)
	}
}
