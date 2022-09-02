/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package coremain

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/metrics"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"go.uber.org/zap"
	"reflect"
	"sync"
)

// NewPluginArgsFunc represents a func that creates a new args object.
type NewPluginArgsFunc func() interface{}

// NewPluginFunc represents a func that can init a Plugin.
// args is the object created by NewPluginArgsFunc.
type NewPluginFunc func(bp *BP, args interface{}) (p Plugin, err error)

type PluginTypeInfo struct {
	NewPlugin NewPluginFunc
	NewArgs   NewPluginArgsFunc
}

var (
	// pluginTypeRegister stores init funcs for certain plugin types
	pluginTypeRegister struct {
		sync.RWMutex
		m map[string]PluginTypeInfo
	}
)

// RegNewPluginFunc registers the type.
// If the type has been registered. RegNewPluginFunc will panic.
func RegNewPluginFunc(typ string, initFunc NewPluginFunc, argsType NewPluginArgsFunc) {
	pluginTypeRegister.Lock()
	defer pluginTypeRegister.Unlock()

	_, ok := pluginTypeRegister.m[typ]
	if ok {
		panic(fmt.Sprintf("duplicate plugin type [%s]", typ))
	}

	if pluginTypeRegister.m == nil {
		pluginTypeRegister.m = make(map[string]PluginTypeInfo)
	}
	pluginTypeRegister.m[typ] = PluginTypeInfo{
		NewPlugin: initFunc,
		NewArgs:   argsType,
	}
}

// DelPluginType deletes the init func for this plugin type.
// It is a noop if pluginType is not registered.
func DelPluginType(typ string) {
	pluginTypeRegister.Lock()
	defer pluginTypeRegister.Unlock()
	delete(pluginTypeRegister.m, typ)
}

// GetPluginType gets the registered type init func.
func GetPluginType(typ string) (PluginTypeInfo, bool) {
	pluginTypeRegister.RLock()
	defer pluginTypeRegister.RUnlock()

	info, ok := pluginTypeRegister.m[typ]
	return info, ok
}

// NewPlugin initialize a Plugin from c.
func NewPlugin(c *PluginConfig, lg *zap.Logger, m *Mosdns) (p Plugin, err error) {
	typeInfo, ok := GetPluginType(c.Type)
	if !ok {
		return nil, fmt.Errorf("plugin type %s not defined", c.Type)
	}

	bp := NewBP(c.Tag, c.Type, lg, m)

	// parse args
	if typeInfo.NewArgs != nil {
		args := typeInfo.NewArgs()
		if m, ok := c.Args.(map[string]interface{}); ok {
			if err = utils.WeakDecode(m, args); err != nil {
				return nil, fmt.Errorf("unable to decode plugin args: %w", err)
			}
		} else {
			tc := reflect.TypeOf(c.Args) // args type from config
			tp := reflect.TypeOf(args)   // args type from plugin init func
			if tc == tp {
				args = c.Args
			} else {
				return nil, fmt.Errorf("invalid plugin args type, want %s, got %s", tp.String(), tc.String())
			}
		}

		return typeInfo.NewPlugin(bp, args)
	}

	return typeInfo.NewPlugin(bp, c.Args)
}

// GetAllPluginTypes returns all plugin types which are configurable.
func GetAllPluginTypes() []string {
	pluginTypeRegister.RLock()
	defer pluginTypeRegister.RUnlock()

	var t []string
	for typ := range pluginTypeRegister.m {
		t = append(t, typ)
	}
	return t
}

type NewPersetPluginFunc func(bp *BP) (Plugin, error)

var presetPluginFuncReg struct {
	sync.Mutex
	m map[string]NewPersetPluginFunc
}

func RegNewPersetPluginFunc(tag string, f NewPersetPluginFunc) {
	presetPluginFuncReg.Lock()
	defer presetPluginFuncReg.Unlock()
	if _, ok := presetPluginFuncReg.m[tag]; ok {
		panic(fmt.Sprintf("preset plugin %s has already been registered", tag))
	}
	if presetPluginFuncReg.m == nil {
		presetPluginFuncReg.m = make(map[string]NewPersetPluginFunc)
	}
	presetPluginFuncReg.m[tag] = f
}

func LoadNewPersetPluginFuncs() map[string]NewPersetPluginFunc {
	presetPluginFuncReg.Lock()
	defer presetPluginFuncReg.Unlock()
	m := make(map[string]NewPersetPluginFunc)
	for tag, pluginFunc := range presetPluginFuncReg.m {
		m[tag] = pluginFunc
	}
	return m
}

// BP represents a basic plugin, which implements Plugin.
// It also has an internal logger, for convenience.
type BP struct {
	Metrics metrics.Registry

	tag, typ string

	l *zap.Logger
	s *zap.SugaredLogger

	m          *Mosdns
	metricsReg *metrics.Registry
}

// NewBP creates a new BP and initials its logger.
func NewBP(tag string, typ string, lg *zap.Logger, m *Mosdns) *BP {
	if lg == nil {
		lg = zap.NewNop()
	}
	return &BP{tag: tag, typ: typ, l: lg, s: lg.Sugar(), m: m}
}

func (p *BP) Tag() string {
	return p.tag
}

func (p *BP) Type() string {
	return p.typ
}

func (p *BP) L() *zap.Logger {
	return p.l
}

func (p *BP) S() *zap.SugaredLogger {
	return p.s
}

func (p *BP) M() *Mosdns {
	return p.m
}

func (p *BP) GetMetricsReg() *metrics.Registry {
	return p.m.pluginsMetricsReg.GetOrSet(p.tag, func() metrics.Var {
		return metrics.NewRegistry()
	}).(*metrics.Registry)
}

func (p *BP) Close() error {
	return nil
}
