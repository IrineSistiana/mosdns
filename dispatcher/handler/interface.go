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

import "context"

// Plugin represents the basic plugin.
type Plugin interface {
	Tag() string
	Type() string
}

// Executable can do some cool stuffs.
type Executable interface {
	Exec(ctx context.Context, qCtx *Context) (err error)
}

// ExecutablePlugin: See Executable.
type ExecutablePlugin interface {
	Plugin
	Executable
}

// ESExecutable: Early Stoppable Executable.
type ESExecutable interface {
	// ExecES: Execute something. earlyStop indicates that it wants
	// to stop the utils.ExecutableCmdSequence ASAP.
	ExecES(ctx context.Context, qCtx *Context) (earlyStop bool, err error)
}

// ESExecutablePlugin: See ESExecutable.
type ESExecutablePlugin interface {
	Plugin
	ESExecutable
}

// Matcher represents a matcher that can match certain patten in Context.
type Matcher interface {
	Match(ctx context.Context, qCtx *Context) (matched bool, err error)
}

// MatcherPlugin: See Matcher.
type MatcherPlugin interface {
	Plugin
	Matcher
}

// ContextConnector can choose when and how to execute its successor.
type ContextConnector interface {
	// Connect connects this ContextPlugin to its predecessor.
	Connect(ctx context.Context, qCtx *Context, pipeCtx *PipeContext) (err error)
}

// ContextPlugin: See ContextConnector.
type ContextPlugin interface {
	Plugin
	ContextConnector
}

// Service represents a background service.
type Service interface {
	// Shutdown and release resources.
	Shutdown() error
}

// ServicePlugin: See Service.
type ServicePlugin interface {
	Plugin
	Service
}
