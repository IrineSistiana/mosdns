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
	"reflect"
)

type NewPluginFunc func(tag string, args Args) (p Plugin, err error)

var (
	// pluginTypeRegister stores init funcs for all plugin types
	pluginTypeRegister = make(map[string]NewPluginFunc)

	functionalPluginRegister = make(map[string]FunctionalPlugin)
	matcherPluginRegister    = make(map[string]MatcherPlugin)
	sequencePluginRegister   = make(map[string]RouterPlugin)
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
	newPluginFunc, ok := pluginTypeRegister[c.Type]
	if !ok {
		return nil, NewTypeNotDefinedErr(c.Type)
	}

	return newPluginFunc(c.Tag, c.Args)
}

// RegPlugin registers this Plugin globally.
// Duplicate Plugin tag will be overwritten.
// Plugin must be a FunctionalPlugin, MatcherPlugin or RouterPlugin.
func RegPlugin(p Plugin) error {
	switch e := p.(type) {
	case FunctionalPlugin:
		functionalPluginRegister[p.Tag()] = e
	case MatcherPlugin:
		matcherPluginRegister[p.Tag()] = e
	case RouterPlugin:
		sequencePluginRegister[p.Tag()] = e
	default:
		return fmt.Errorf("unexpected plugin interface type %s", reflect.TypeOf(p).Name())
	}
	return nil
}

func GetFunctionalPlugin(tag string) (p FunctionalPlugin, ok bool) {
	p, ok = functionalPluginRegister[tag]
	return
}

func GetMatcherPlugin(tag string) (p MatcherPlugin, ok bool) {
	p, ok = matcherPluginRegister[tag]
	return
}

func GetRouterPlugin(tag string) (p RouterPlugin, ok bool) {
	p, ok = sequencePluginRegister[tag]
	return
}

const (
	// IterationLimit is to prevent endless loops.
	IterationLimit = 50

	// StopSignTag: See Walk().
	StopSignTag = "end"
)

// Walk walks into this RouterPlugin. Walk will stop and return when
// last RouterPlugin.Do() returns:
// 1. An empty tag or StopSignTag.
// 2. An error.
func Walk(ctx context.Context, qCtx *Context, entryTag string) (err error) {
	nextTag := entryTag

	for i := 0; i < IterationLimit; i++ {
		p, ok := GetRouterPlugin(nextTag) // get next plugin
		if !ok {
			return NewTagNotDefinedErr(nextTag)
		}
		logger.GetStd().Debugf("%v: exec plugin %s", qCtx, p.Tag())

		nextTag, err = p.Do(ctx, qCtx)
		if err != nil {
			return fmt.Errorf("plugin %s reports an err: %w", p.Tag(), err)
		}
		if len(nextTag) == 0 || nextTag == StopSignTag { // end of the plugin chan
			return nil
		}
	}

	return fmt.Errorf("length of plugin execution sequence reached limit %d", IterationLimit)
}
