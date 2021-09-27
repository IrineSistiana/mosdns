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

// Executable represents something that is executable.
type Executable interface {
	Exec(ctx context.Context, qCtx *Context, next ExecutableChainNode) error
}

type ExecutableChainNode interface {
	Executable
	linkedList
}

// ExecutablePlugin: See ESExecutable.
type ExecutablePlugin interface {
	Plugin
	Executable
}

// Matcher represents a matcher that can match a certain patten in Context.
type Matcher interface {
	Match(ctx context.Context, qCtx *Context) (matched bool, err error)
}

// MatcherPlugin: See Matcher.
type MatcherPlugin interface {
	Plugin
	Matcher
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

type ExecutableNodeWrapper struct {
	Executable
	NodeLinker
}

func WarpExecutable(e Executable) ExecutableChainNode {
	return &ExecutableNodeWrapper{Executable: e}
}

type linkedList interface {
	Previous() ExecutableChainNode
	Next() ExecutableChainNode
	LinkPrevious(n ExecutableChainNode)
	LinkNext(n ExecutableChainNode)
}

type NodeLinker struct {
	prev, next ExecutableChainNode
}

func (l *NodeLinker) Previous() ExecutableChainNode {
	return l.prev
}

func (l *NodeLinker) Next() ExecutableChainNode {
	return l.next
}

func (l *NodeLinker) LinkPrevious(n ExecutableChainNode) {
	l.prev = n
}

func (l *NodeLinker) LinkNext(n ExecutableChainNode) {
	l.next = n
}
