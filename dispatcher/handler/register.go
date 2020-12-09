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
	"fmt"
	"sync"
)

type NewPluginFunc func(tag string, args map[string]interface{}) (p Plugin, err error)

var (
	// configurablePluginTypeRegister stores init funcs for certain plugin types
	configurablePluginTypeRegister = make(map[string]NewPluginFunc)

	pluginTagRegister = newPluginRegister()
)

type pluginRegister struct {
	sync.RWMutex
	register map[string]Plugin
}

func newPluginRegister() *pluginRegister {
	return &pluginRegister{
		register: make(map[string]Plugin),
	}
}

func (r *pluginRegister) regPlugin(p Plugin, overwrite bool) error {
	r.Lock()
	defer r.Unlock()

	if !overwrite {
		if _, ok := r.register[p.Tag()]; ok {
			return fmt.Errorf("plugin %s has been registered", p.Tag())
		}
	}
	r.register[p.Tag()] = p

	return nil
}

func (r *pluginRegister) getFunctionalPlugin(tag string) (p FunctionalPlugin, ok bool) {
	r.RLock()
	defer r.RUnlock()
	if gp, ok := r.register[tag]; ok {
		if p, ok := gp.(FunctionalPlugin); ok {
			return p, true
		}
	}

	return
}
func (r *pluginRegister) getMatcherPlugin(tag string) (p MatcherPlugin, ok bool) {
	r.RLock()
	defer r.RUnlock()
	if gp, ok := r.register[tag]; ok {
		if p, ok := gp.(MatcherPlugin); ok {
			return p, true
		}
	}
	return
}

func (r *pluginRegister) getRouterPlugin(tag string) (p RouterPlugin, ok bool) {
	r.RLock()
	defer r.RUnlock()
	if gp, ok := r.register[tag]; ok {
		if p, ok := gp.(RouterPlugin); ok {
			return p, true
		}
	}
	return
}

func (r *pluginRegister) getPlugin(tag string) (p Plugin, ok bool) {
	r.RLock()
	defer r.RUnlock()
	p, ok = r.register[tag]
	return
}

func (r *pluginRegister) getAllPluginTag() []string {
	r.RLock()
	defer r.RUnlock()

	t := make([]string, 0, len(r.register))
	for tag := range r.register {
		t = append(t, tag)
	}
	return t
}

func (r *pluginRegister) purge() {
	r.Lock()
	r.register = make(map[string]Plugin)
	r.Unlock()
}

// RegInitFunc registers this plugin type.
// This should only be called in init() of the plugin package.
// Duplicate plugin types are not allowed.
func RegInitFunc(pluginType string, initFunc NewPluginFunc) {
	_, ok := configurablePluginTypeRegister[pluginType]
	if ok {
		panic(fmt.Sprintf("duplicate plugin type [%s]", pluginType))
	}
	configurablePluginTypeRegister[pluginType] = initFunc
}

// GetConfigurablePluginTypes returns all plugin types which are configurable.
func GetConfigurablePluginTypes() []string {
	b := make([]string, 0, len(configurablePluginTypeRegister))
	for typ := range configurablePluginTypeRegister {
		b = append(b, typ)
	}
	return b
}

// InitAndRegPlugin inits and registers this plugin globally.
// Duplicate plugin tags are not allowed.
func InitAndRegPlugin(c *Config) (err error) {
	p, err := NewPlugin(c)
	if err != nil {
		return fmt.Errorf("failed to init plugin [%s], %w", c.Tag, err)
	}

	return RegPlugin(p)
}

func NewPlugin(c *Config) (p Plugin, err error) {
	newPluginFunc, ok := configurablePluginTypeRegister[c.Type]
	if !ok {
		return nil, NewErrFromTemplate(ETTypeNotDefined, c.Type)
	}

	return newPluginFunc(c.Tag, c.Args)
}

// RegPlugin registers this Plugin globally.
// Duplicate Plugin tag will cause an error.
func RegPlugin(p Plugin) error {
	return pluginTagRegister.regPlugin(p, false)
}

// RegPlugin registers this Plugin globally.
// Duplicate Plugin tag will be overwritten.
func RegPluginOverwrite(p Plugin) error {
	return pluginTagRegister.regPlugin(p, true)
}

// MustRegPlugin: see RegPlugin.
// MustRegPlugin will panic if err.
func MustRegPlugin(p Plugin) {
	err := pluginTagRegister.regPlugin(p, false)
	if err != nil {
		panic(err.Error())
	}
}

func GetPlugin(tag string) (p Plugin, ok bool) {
	return pluginTagRegister.getPlugin(tag)
}

func GetAllPluginTag() []string {
	return pluginTagRegister.getAllPluginTag()
}

func GetFunctionalPlugin(tag string) (p FunctionalPlugin, ok bool) {
	return pluginTagRegister.getFunctionalPlugin(tag)
}

func GetMatcherPlugin(tag string) (p MatcherPlugin, ok bool) {
	return pluginTagRegister.getMatcherPlugin(tag)
}

func GetRouterPlugin(tag string) (p RouterPlugin, ok bool) {
	return pluginTagRegister.getRouterPlugin(tag)
}

// PurgePluginRegister should only be used in test.
func PurgePluginRegister() {
	pluginTagRegister.purge()
}
