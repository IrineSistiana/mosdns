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
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"io"
)

// Plugin represents the basic plugin.
type Plugin interface {
	Tag() string
	Type() string
	io.Closer
}

// ExecutablePlugin represents a Plugin that is Executable.
type ExecutablePlugin interface {
	Plugin
	executable_seq.Executable
}

// MatcherPlugin represents a Plugin that is a Matcher.
type MatcherPlugin interface {
	Plugin
	executable_seq.Matcher
}
