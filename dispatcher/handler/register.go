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
	"github.com/IrineSistiana/mosdns/v2/dispatcher/mlog"
	"go.uber.org/zap"
	"sync"
)

// NewArgsFunc represents a func that creates a new args object.
type NewArgsFunc func() interface{}

// NewPluginFunc represents a func that can init a Plugin.
// args is the object created by NewArgsFunc.
type NewPluginFunc func(bp *BP, args interface{}) (p Plugin, err error)

type TypeInfo struct {
	NewPlugin NewPluginFunc
	NewArgs   NewArgsFunc
}

var (
	// typeRegister stores init funcs for certain plugin types
	typeRegister struct {
		sync.RWMutex
		m map[string]TypeInfo
	}

	tagRegister struct {
		sync.RWMutex
		m map[string]*PluginWrapper
	}
)

// RegInitFunc registers the type.
// If the type has been registered. RegInitFunc will panic.
func RegInitFunc(typ string, initFunc NewPluginFunc, argsType NewArgsFunc) {
	typeRegister.Lock()
	defer typeRegister.Unlock()

	_, ok := typeRegister.m[typ]
	if ok {
		panic(fmt.Sprintf("duplicate plugin type [%s]", typ))
	}

	if typeRegister.m == nil {
		typeRegister.m = make(map[string]TypeInfo)
	}
	typeRegister.m[typ] = TypeInfo{
		NewPlugin: initFunc,
		NewArgs:   argsType,
	}
}

// DelInitFunc deletes the init func for this plugin type.
// It is a noop if pluginType is not registered.
func DelInitFunc(typ string) {
	typeRegister.Lock()
	defer typeRegister.Unlock()
	delete(typeRegister.m, typ)
}

// GetInitFunc gets the registered type init func.
func GetInitFunc(typ string) (TypeInfo, bool) {
	typeRegister.RLock()
	defer typeRegister.RUnlock()

	info, ok := typeRegister.m[typ]
	return info, ok
}

// NewPlugin initialize a Plugin from c.
func NewPlugin(c *Config) (p Plugin, err error) {
	typeInfo, ok := GetInitFunc(c.Type)
	if !ok {
		return nil, fmt.Errorf("plugin type %s not defined", c.Type)
	}

	bp := NewBP(c.Tag, c.Type)

	// parse args
	if typeInfo.NewArgs != nil {
		args := typeInfo.NewArgs()
		err = WeakDecode(c.Args, args)
		if err != nil {
			return nil, fmt.Errorf("unable to decode plugin args: %w", err)
		}
		return typeInfo.NewPlugin(bp, args)
	}

	return typeInfo.NewPlugin(bp, c.Args)
}

// RegPlugin registers Plugin p.
// RegPlugin will not register p and returns false if the tag of p
// has already been registered.
func RegPlugin(p Plugin) bool {
	tagRegister.Lock()
	defer tagRegister.Unlock()

	if tagRegister.m == nil {
		tagRegister.m = make(map[string]*PluginWrapper)
	}

	tag := p.Tag()
	_, dup := tagRegister.m[tag]
	if dup {
		return false
	}
	tagRegister.m[tag] = NewPluginWrapper(p)
	return true
}

// MustRegPlugin will panic the tag of p has already been registered.
func MustRegPlugin(p Plugin) {
	if !RegPlugin(p) {
		panic(fmt.Sprintf("tag [%s] has already been registered", p.Tag()))
	}
}

// GetPlugin gets a registered PluginWrapper.
// Also see PluginWrapper.
func GetPlugin(tag string) (p *PluginWrapper) {
	tagRegister.RLock()
	defer tagRegister.RUnlock()
	return tagRegister.m[tag]
}

// DelPlugin deletes this plugin tag.
// It is a noop if tag is not registered.
func DelPlugin(tag string) {
	tagRegister.Lock()
	defer tagRegister.Unlock()

	delete(tagRegister.m, tag)
}

// GetPluginAll returns all registered plugins.
func GetPluginAll() []Plugin {
	tagRegister.RLock()
	defer tagRegister.RUnlock()

	var p []Plugin
	for _, pw := range tagRegister.m {
		p = append(p, pw.Plugin)
	}
	return p
}

// GetConfigurablePluginTypes returns all plugin types which are configurable.
func GetConfigurablePluginTypes() []string {
	typeRegister.RLock()
	defer typeRegister.RUnlock()

	var t []string
	for typ := range typeRegister.m {
		t = append(t, typ)
	}
	return t
}

// PurgePluginRegister should only be used in testing.
func PurgePluginRegister() {
	tagRegister.Lock()
	tagRegister.m = make(map[string]*PluginWrapper)
	tagRegister.Unlock()
}

// BP represents a basic plugin, which implements Plugin.
// It also has an internal logger, for convenience.
type BP struct {
	tag, typ string

	l *zap.Logger
	s *zap.SugaredLogger
}

// NewBP creates a new BP and initials its logger.
func NewBP(tag string, typ string) *BP {
	l := mlog.NewPluginLogger(tag)
	return &BP{tag: tag, typ: typ, l: l, s: l.Sugar()}
}

func (p *BP) Tag() string {
	return p.tag
}

func (p *BP) Type() string {
	return p.typ
}

func (p *BP) Shutdown() error {
	return nil
}

func (p *BP) L() *zap.Logger {
	return p.l
}

func (p *BP) S() *zap.SugaredLogger {
	return p.s
}
