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
	Shutdown() error
}

// Executable represents something that is executable.
type Executable interface {
	Exec(ctx context.Context, qCtx *Context, next ExecutableChainNode) error
}

// ExecutableChainNode represents a node in a executable chain.
type ExecutableChainNode interface {
	Executable
	LinkedListNode
}

// ExecutablePlugin represents a Plugin that is Executable.
type ExecutablePlugin interface {
	Plugin
	Executable
}

// Matcher represents a matcher that can match a certain patten in Context.
type Matcher interface {
	Match(ctx context.Context, qCtx *Context) (matched bool, err error)
}

// MatcherPlugin represents a Plugin that is a Matcher.
type MatcherPlugin interface {
	Plugin
	Matcher
}

// ExecutableNodeWrapper wraps a Executable to a ExecutableChainNode.
type ExecutableNodeWrapper struct {
	Executable
	NodeLinker
}

// WrapExecutable wraps a Executable to a ExecutableChainNode.
func WrapExecutable(e Executable) ExecutableChainNode {
	if ecn, ok := e.(ExecutableChainNode); ok {
		return ecn
	}
	return &ExecutableNodeWrapper{Executable: e}
}

type LinkedListNode interface {
	Previous() ExecutableChainNode
	Next() ExecutableChainNode
	LinkPrevious(n ExecutableChainNode)
	LinkNext(n ExecutableChainNode)
}

// NodeLinker implements LinkedListNode.
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
