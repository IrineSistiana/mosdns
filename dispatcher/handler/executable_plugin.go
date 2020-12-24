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

type ExecutablePlugin interface {
	Plugin
	Executable
}

type Executable interface {
	Exec(ctx context.Context, qCtx *Context) (err error)
}

type ExecutablePluginWrapper struct {
	tag string
	typ string

	Executable
}

func (p *ExecutablePluginWrapper) Tag() string {
	return p.tag
}

func (p *ExecutablePluginWrapper) Type() string {
	return p.typ
}

// WrapExecutablePlugin returns a *ExecutablePluginWrapper which implements Plugin and ExecutablePlugin.
func WrapExecutablePlugin(tag, typ string, executable Executable) *ExecutablePluginWrapper {
	return &ExecutablePluginWrapper{
		tag:        tag,
		typ:        typ,
		Executable: executable,
	}
}
