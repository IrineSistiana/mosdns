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

package handler

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"go.uber.org/zap"
	"sync"
)

type NewPluginFunc func(bp *BP, args interface{}) (p Plugin, err error)
type NewArgsFunc func() interface{}

type typeInfo struct {
	newPlugin NewPluginFunc
	newArgs   NewArgsFunc
}

var (
	// pluginTypeRegister stores init funcs for certain plugin types
	pluginTypeRegister = make(map[string]typeInfo)

	pluginTagRegister = newPluginRegister()
)

type pluginRegister struct {
	sync.RWMutex
	register map[string]*PluginWrapper
}

func newPluginRegister() *pluginRegister {
	return &pluginRegister{
		register: make(map[string]*PluginWrapper),
	}
}

// regPlugin registers p, if errIfDup is true and p.Tag is duplicated, an err will be returned.
// If old plugin is a ServicePlugin, regPlugin will call ServicePlugin.Shutdown(). If it failed to
// shutdown the old service, it will panic.
func (r *pluginRegister) regPlugin(p Plugin, errIfDup bool) error {
	r.Lock()

	tag := p.Tag()
	oldWrapper, dup := r.register[tag]

	if dup && errIfDup {
		r.Unlock()
		return fmt.Errorf("plugin tag %s has been registered", tag)
	}
	r.register[tag] = newPluginWrapper(p)
	r.Unlock()

	if dup {
		mlog.L().Info("plugin overwritten", zap.String("tag", tag))
		r.tryShutdownService(oldWrapper)
	}
	return nil
}

func (r *pluginRegister) getPlugin(tag string) (p *PluginWrapper, err error) {
	r.RLock()
	defer r.RUnlock()
	p, ok := r.register[tag]
	if !ok {
		return nil, fmt.Errorf("plugin tag %s not defined", tag)
	}
	return p, nil
}

func (r *pluginRegister) delPlugin(tag string) {
	r.Lock()
	p, ok := r.register[tag]
	if !ok {
		r.Unlock()
		return
	}
	delete(r.register, tag)
	r.Unlock()

	r.tryShutdownService(p)
	return
}

func (r *pluginRegister) tryShutdownService(oldWrapper *PluginWrapper) {
	tag := oldWrapper.GetPlugin().Tag()
	if oldWrapper.Is(PITService) {
		mlog.L().Info("shutting down old service", zap.String("tag", tag))
		if err := oldWrapper.Shutdown(); err != nil {
			panic(fmt.Sprintf("service %s failed to shutdown: %v", tag, err))
		}
		mlog.L().Info("old service exited", zap.String("tag", tag))
	}
}

func (r *pluginRegister) getPluginAll() []Plugin {
	r.RLock()
	defer r.RUnlock()

	t := make([]Plugin, 0, len(r.register))
	for _, pw := range r.register {
		t = append(t, pw.GetPlugin())
	}
	return t
}

func (r *pluginRegister) purge() {
	r.Lock()
	r.register = make(map[string]*PluginWrapper)
	r.Unlock()
}

// RegInitFunc registers this plugin type.
// This should only be called in init() from the plugin package.
// Duplicate plugin types are not allowed.
func RegInitFunc(pluginType string, initFunc NewPluginFunc, argsType NewArgsFunc) {
	_, ok := pluginTypeRegister[pluginType]
	if ok {
		panic(fmt.Sprintf("duplicate plugin type [%s]", pluginType))
	}
	pluginTypeRegister[pluginType] = typeInfo{
		newPlugin: initFunc,
		newArgs:   argsType,
	}
}

// InitAndRegPlugin inits and registers this plugin globally.
// This is a help func of NewPlugin + RegPlugin.
func InitAndRegPlugin(c *Config, errIfDup bool) (err error) {
	p, err := NewPlugin(c)
	if err != nil {
		return fmt.Errorf("failed to init plugin [%s], %w", c.Tag, err)
	}

	return RegPlugin(p, errIfDup)
}

func NewPlugin(c *Config) (p Plugin, err error) {
	typeInfo, ok := pluginTypeRegister[c.Type]
	if !ok {
		return nil, fmt.Errorf("plugin type %s not defined", c.Type)
	}

	bp := NewBP(c.Tag, c.Type)

	// parse args
	if typeInfo.newArgs != nil {
		args := typeInfo.newArgs()
		err = WeakDecode(c.Args, args)
		if err != nil {
			return nil, fmt.Errorf("unable to decode plugin args: %w", err)
		}
		return typeInfo.newPlugin(bp, args)
	}

	return typeInfo.newPlugin(bp, c.Args)
}

// RegPlugin registers this Plugin globally.
// Duplicate Plugin tag will overwrite the old one.
func RegPlugin(p Plugin, errIfDup bool) error {
	return pluginTagRegister.regPlugin(p, errIfDup)
}

// MustRegPlugin: see RegPlugin.
// MustRegPlugin will panic if any err occurred.
func MustRegPlugin(p Plugin, errIfDup bool) {
	err := pluginTagRegister.regPlugin(p, errIfDup)
	if err != nil {
		panic(err.Error())
	}
}

func GetPlugin(tag string) (p *PluginWrapper, err error) {
	return pluginTagRegister.getPlugin(tag)
}

// DelPlugin deletes this plugin.
// If this plugin is a Service, DelPlugin will call Service.Shutdown().
// DelPlugin will panic if Service.Shutdown() returns an err.
func DelPlugin(tag string) {
	pluginTagRegister.delPlugin(tag)
}

// GetPluginAll returns all registered plugins.
// This should only be used in test or debug.
func GetPluginAll() []Plugin {
	return pluginTagRegister.getPluginAll()
}

// GetConfigurablePluginTypes returns all plugin types which are configurable.
// This should only be used in test or debug.
func GetConfigurablePluginTypes() []string {
	b := make([]string, 0, len(pluginTypeRegister))
	for typ := range pluginTypeRegister {
		b = append(b, typ)
	}
	return b
}

// PurgePluginRegister should only be used in test.
func PurgePluginRegister() {
	pluginTagRegister.purge()
}
