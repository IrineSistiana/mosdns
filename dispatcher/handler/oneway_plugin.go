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

import "context"

// OneWayPlugin only modifies qCtx
type OneWayPlugin interface {
	Modify(ctx context.Context, qCtx *Context) (err error)
}

type oneWayPluginWrapper struct {
	config   *Config
	doPlugin OneWayPlugin
	next     string
}

func (p *oneWayPluginWrapper) Tag() string {
	return p.config.Tag
}

func (p *oneWayPluginWrapper) Type() string {
	return p.config.Type
}

func (p *oneWayPluginWrapper) Do(ctx context.Context, qCtx *Context) (next string, err error) {
	err = p.doPlugin.Modify(ctx, qCtx)
	return p.next, err
}

// WrapOneWayPlugin returns a oneWayPluginWrapper which implements Plugin.
func WrapOneWayPlugin(config *Config, doPlugin OneWayPlugin, next string) Plugin {
	return &oneWayPluginWrapper{
		config:   config,
		doPlugin: doPlugin,
		next:     next,
	}
}
