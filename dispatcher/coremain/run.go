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

package coremain

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin"
	"os"
	"os/signal"
	"syscall"
)

// Run starts mosdns, it blocks.
func Run(c string) {
	//wait for signals
	go func() {
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
		s := <-osSignals
		mlog.Entry().Infof("received signal: %v, bye", s)
		os.Exit(0)
	}()

	err := loadConfig(c, 0)
	if err != nil {
		mlog.Entry().Fatal(err)
	}

	mlog.Entry().Info("all plugins are successfully loaded")
	mlog.Entry().Debugf("loaded plugins: %v", handler.GetAllPluginTag())
	select {}
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

	mlog.Entry().Infof("loading config file: %s", f)
	c, err := parseConfig(f)
	if err != nil {
		return err
	}

	for i, pluginConfig := range c.Plugin {
		if len(pluginConfig.Tag) == 0 {
			mlog.Entry().Warnf("plugin at index %d has a empty tag, ignore it", i)
			continue
		}
		if err := handler.InitAndRegPlugin(pluginConfig); err != nil {
			return fmt.Errorf("failed to register plugin %d %s: %w", i, pluginConfig.Tag, err)
		}
		mlog.Entry().Infof("plugin %s loaded", pluginConfig.Tag)
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
