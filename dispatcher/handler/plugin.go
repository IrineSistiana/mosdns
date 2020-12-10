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

import "github.com/IrineSistiana/mosdns/dispatcher/mlog"

type Plugin interface {
	Tag() string
	Type() string
}

type Config struct {
	// Tag, required
	Tag string `yaml:"tag"`

	// Type, required
	Type string `yaml:"type"`

	// Args, might be required by some plugins
	Args map[string]interface{} `yaml:"args"`
}

// PluginFatalErr: If a plugin has a fatal err, call this.
func PluginFatalErr(tag string, msg string) {
	mlog.Entry().Fatalf("plugin %s reported a fatal err: %s", tag, msg)
}
