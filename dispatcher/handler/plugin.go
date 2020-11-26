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
)

// DoPlugin only modifies qCtx
type DoPlugin interface {
	Do(ctx context.Context, qCtx *Context) (err error)
}

type Plugin interface {
	Tag() string
	Type() string

	DoPlugin

	// Next returns next plugin tag.
	Next(ctx context.Context, qCtx *Context) (next string, err error)
}

type Config struct {
	// Tag, required
	Tag string `yaml:"tag"`

	// Type, required
	Type string `yaml:"type"`

	// Args, might be required by some plugins
	Args Args `yaml:"args"`
}

type doPluginWrapper struct {
	config   *Config
	doPlugin DoPlugin
	next     string
}

func (p *doPluginWrapper) Tag() string {
	return p.config.Tag
}

func (p *doPluginWrapper) Type() string {
	return p.config.Type
}

func (p *doPluginWrapper) Do(ctx context.Context, qCtx *Context) (err error) {
	return p.doPlugin.Do(ctx, qCtx)
}

func (p *doPluginWrapper) Next(ctx context.Context, qCtx *Context) (next string, err error) {
	return p.next, nil
}

// WrapDoPlugin returns a doPluginWrapper which implements Plugin.
func WrapDoPlugin(config *Config, doPlugin DoPlugin, next string) Plugin {
	return &doPluginWrapper{
		config:   config,
		doPlugin: doPlugin,
		next:     next,
	}
}
