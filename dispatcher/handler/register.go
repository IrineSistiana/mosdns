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

// PluginRegister stores user defined plugin
type PluginRegister map[string]Plugin

var (
	// newFuncRegister stores init funcs for all plugin types
	newFuncRegister = make(map[string]NewPluginFunc)
)

// RegInitFunc registers this plugin type.
func RegInitFunc(pluginType string, initFunc NewPluginFunc) {
	_, ok := newFuncRegister[pluginType]
	if ok {
		panic(fmt.Sprintf("duplicate plugin type [%s]", pluginType))
	}
	newFuncRegister[pluginType] = initFunc
}

func GetAllSupportPluginTypes() []string {
	b := make([]string, 0, len(newFuncRegister))
	for typ := range newFuncRegister {
		b = append(b, typ)
	}
	return b
}

func NewPluginRegister() PluginRegister {
	return make(PluginRegister)
}

func (reg PluginRegister) RegPlugin(c *Config) (err error) {
	newPluginFunc, ok := newFuncRegister[c.Type]
	if !ok {
		return fmt.Errorf("undefinded plugin type [%s]", c.Type)
	}

	p, err := newPluginFunc(c)
	if err != nil {
		return fmt.Errorf("failed to init plugin [%s], %w", c.Tag, err)
	}
	reg.regPlugin(c.Tag, p)
	return nil
}

func (reg PluginRegister) GetPlugin(tag string) (p Plugin, ok bool) {
	p, ok = reg[tag]
	return
}

func (reg PluginRegister) regPlugin(tag string, p Plugin) {
	_, ok := reg[tag]
	if ok {
		logger.GetStd().Warnf("duplicate plugin tag [%s], overwrite it", tag)
	} else {
		logger.GetStd().Debugf("plugin %s registered", tag)
	}
	reg[tag] = p
}

func (reg PluginRegister) Walk(ctx context.Context, qCtx *Context, entryTag string) (err error) {
	nextTag := entryTag

	for {
		p, ok := reg.GetPlugin(nextTag) // get next plugin
		if !ok {
			return fmt.Errorf("unregisted plugin [%s]", nextTag)
		}
		logger.GetStd().Debugf("%v: enter plugin %s", qCtx, p.Tag())

		if err := p.Do(ctx, qCtx); err != nil {
			return fmt.Errorf("plugin %s Do() reports an err: %w", p.Tag(), err)
		}

		tag, err := p.Next(ctx, qCtx)
		if err != nil {
			return fmt.Errorf("plugin %s Next() reports an err: %w", p.Tag(), err)
		}
		if len(tag) == 0 { // end of the plugin chan
			return nil
		}

		nextTag = tag
	}
}
