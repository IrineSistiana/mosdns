//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) or later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package executable_seq

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"go.uber.org/zap"
)

type Executable interface {
	Exec(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error)
}

type ExecutableNode interface {
	Executable
	linkedList
}

type ExecutableNodeWrapper struct {
	Executable
	NodeLinker
}

func WarpExecutable(e Executable) ExecutableNode {
	return &ExecutableNodeWrapper{Executable: e}
}

type linkedList interface {
	Previous() ExecutableNode
	Next() ExecutableNode
	LinkPrevious(n ExecutableNode)
	LinkNext(n ExecutableNode)
}

type NodeLinker struct {
	prev, next ExecutableNode
}

func (l *NodeLinker) Previous() ExecutableNode {
	return l.prev
}

func (l *NodeLinker) Next() ExecutableNode {
	return l.next
}

func (l *NodeLinker) LinkPrevious(n ExecutableNode) {
	l.prev = n
}

func (l *NodeLinker) LinkNext(n ExecutableNode) {
	l.next = n
}
