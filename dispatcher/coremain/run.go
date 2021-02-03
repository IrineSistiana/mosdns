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
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"plugin"
	"runtime"
	"sync"
	"syscall"
)

// Run starts mosdns, it blocks.
func Run(c string) {
	loadConfig(c, 0)

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
func loadConfig(f string, depth int) {
	if depth >= maxIncludeDepth {
		mlog.S().Fatal("max include depth reached")
	}
	depth++

	mlog.L().Info("loading config", zap.String("file", f))
	c, err := parseConfig(f)
	if err != nil {
		mlog.S().Fatalf("failed to parse config from file %s: %v", f, err)
	}

	if depth == 1 {
		// init logger
		level, err := parseLogLevel(c.Log.Level)
		if err != nil {
			mlog.S().Fatal(err)
		}
		mlog.Level().SetLevel(level)
		if len(c.Log.File) != 0 {
			mlog.L().Info("opening log file", zap.String("file", c.Log.File))
			f, err := os.OpenFile(c.Log.File, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
			if err != nil {
				mlog.S().Fatalf("can not open log file %s: %v", c.Log.File, err)
			}
			mlog.L().Info("redirecting log to file, end of console log", zap.String("file", c.Log.File))
			mlog.Writer().Replace(f)
		}

		for _, lib := range c.Library {
			mlog.L().Info("loading library", zap.String("library", lib))
			_, err := plugin.Open(lib)
			if err != nil {
				mlog.S().Fatalf("failed to open library %s: %v", lib, err)
			}
		}
	}

	n := runtime.NumCPU() / 2
	if n < 1 {
		n = 1
	}
	pool := utils.NewConcurrentLimiter(n)
	wg := new(sync.WaitGroup)
	for i, pluginConfig := range c.Plugin {
		if len(pluginConfig.Tag) == 0 || len(pluginConfig.Type) == 0 {
			continue
		}
		i := i
		pluginConfig := pluginConfig

		select {
		case <-pool.Wait():
			wg.Add(1)
			go func() {
				defer pool.Done()
				defer wg.Done()
				mlog.L().Info("loading plugin", zap.String("tag", pluginConfig.Tag))
				if err := handler.InitAndRegPlugin(pluginConfig, true); err != nil {
					mlog.S().Fatalf("failed to register plugin #%d %s: %v", i, pluginConfig.Tag, err)
				}
			}()
		}
	}
	wg.Wait()

	for _, include := range c.Include {
		if len(include) == 0 {
			continue
		}
		loadConfig(include, depth)
	}
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
