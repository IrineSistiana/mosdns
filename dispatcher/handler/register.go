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

package handler

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/logger"
)

type NewPluginFunc func(c *Config) (p Plugin, err error)

var (
	// pluginTypeRegister stores init funcs for all plugin types
	pluginTypeRegister = make(map[string]NewPluginFunc)

	// pluginRegister stores plugin instance.
	pluginRegister = make(map[string]Plugin)
)

// RegInitFunc registers this plugin type.
// This should only be called in init() of the plugin package.
// Duplicate plugin types are not allowed.
func RegInitFunc(pluginType string, initFunc NewPluginFunc) {
	_, ok := pluginTypeRegister[pluginType]
	if ok {
		panic(fmt.Sprintf("duplicate plugin type [%s]", pluginType))
	}
	pluginTypeRegister[pluginType] = initFunc
}

// GetPluginTypes returns all registered plugin types.
func GetPluginTypes() []string {
	b := make([]string, 0, len(pluginTypeRegister))
	for typ := range pluginTypeRegister {
		b = append(b, typ)
	}
	return b
}

// RegPlugin inits and registers this plugin globally.
// Duplicate plugin tags are not allowed.
func RegPlugin(c *Config) (err error) {
	newPluginFunc, ok := pluginTypeRegister[c.Type]
	if !ok {
		return NewTypeNotDefinedErr(c.Type)
	}

	p, err := newPluginFunc(c)
	if err != nil {
		return fmt.Errorf("failed to init plugin [%s], %w", c.Tag, err)
	}

	_, ok = pluginRegister[c.Tag]
	if ok {
		return fmt.Errorf("duplicate plugin tag [%s]", c.Tag)
	} else {
		logger.GetStd().Debugf("plugin %s registered", c.Tag)
	}
	pluginRegister[c.Tag] = p

	return nil
}

func GetPlugin(tag string) (p Plugin, ok bool) {
	p, ok = pluginRegister[tag]
	return
}

// Walk walks through plugins. Walk will return if last plugin.Next() returns a empty tag or any error occurs.
func Walk(ctx context.Context, qCtx *Context, entryTag string) (err error) {
	nextTag := entryTag

	for {
		p, ok := GetPlugin(nextTag) // get next plugin
		if !ok {
			return NewTagNotDefinedErr(nextTag)
		}
		logger.GetStd().Debugf("%v: enter plugin %s", qCtx, p.Tag())

		nextTag, err = p.Do(ctx, qCtx)
		if err != nil {
			return fmt.Errorf("plugin %s Do() reports an err: %w", p.Tag(), err)
		}
		if len(nextTag) == 0 { // end of the plugin chan
			return nil
		}
	}
}
