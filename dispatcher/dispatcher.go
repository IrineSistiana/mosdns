//     Copyright (C) 2020, IrineSistiana
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

package dispatcher

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/config"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/IrineSistiana/mosdns/dispatcher/logger"
)

// Dispatcher represents a dns query dispatcher
type Dispatcher struct {
	config *config.Config
}

// Init inits a dispatcher from configuration
func Init(c *config.Config) error {
	// init logger
	if len(c.Log.Level) != 0 {
		level, err := logrus.ParseLevel(c.Log.Level)
		if err != nil {
			return err
		}
		logger.GetLogger().SetLevel(level)
	}
	if len(c.Log.File) != 0 {
		f, err := os.OpenFile(c.Log.File, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			return fmt.Errorf("can not open log file %s: %w", c.Log.File, err)
		}
		logger.Entry().Infof("use log file %s", c.Log.File)
		logWriter := io.MultiWriter(os.Stdout, f)
		logger.GetLogger().SetOutput(logWriter)
	}
	if logger.GetLogger().IsLevelEnabled(logrus.DebugLevel) {
		logger.GetLogger().SetReportCaller(true)
		go func() {
			m := new(runtime.MemStats)
			for {
				time.Sleep(time.Second * 15)
				runtime.ReadMemStats(m)
				logger.Entry().Debugf("HeapObjects: %d NumGC: %d PauseTotalNs: %d, NumGoroutine: %d", m.HeapObjects, m.NumGC, m.PauseTotalNs, runtime.NumGoroutine())
			}
		}()
	}

	d := new(Dispatcher)
	d.config = c

	for i, pluginConfig := range c.Plugin {
		if len(pluginConfig.Tag) == 0 {
			logger.Entry().Warnf("plugin at index %d has a empty tag, ignore it", i)
			continue
		}
		if err := handler.InitAndRegPlugin(pluginConfig); err != nil {
			return fmt.Errorf("failed to register plugin %d-%s: %w", i, pluginConfig.Tag, err)
		}
		logger.Entry().Debugf("plugin %s loaded", pluginConfig.Tag)
	}

	return nil
}
