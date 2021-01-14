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

type Executable interface {
	Exec(ctx context.Context, qCtx *Context) (err error)
}

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

type ESExecutablePlugin interface {
	Plugin
	ESExecutable
}

type Matcher interface {
	Match(ctx context.Context, qCtx *Context) (matched bool, err error)
}

type MatcherPlugin interface {
	Plugin
	Matcher
}

type ContextConnector interface {
	// Connect connects this ContextPlugin to its predecessor.
	Connect(ctx context.Context, qCtx *Context, pipeCtx *PipeContext) (err error)
}

type ContextPlugin interface {
	Plugin
	ContextConnector
}

type Service interface {
	// Shutdown and release resources.
	Shutdown() error
}

// ServicePlugin is a plugin that has one or more background tasks that will keep running after Init().
type ServicePlugin interface {
	Plugin
	Service
}
