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

package handler

import "context"

func ExecChainNode(ctx context.Context, qCtx *Context, n ExecutableChainNode) error {
	if n == nil {
		return nil
	}

	// TODO: Error logging
	return n.Exec(ctx, qCtx, n.Next())
}

// FirstNode returns the first node of chain of n.
func FirstNode(n ExecutableChainNode) ExecutableChainNode {
	for {
		p := n.Previous()
		if p == nil {
			return n
		}
		n = p
	}
}

// LatestNode returns the Latest node of chain of n.
func LatestNode(n ExecutableChainNode) ExecutableChainNode {
	for {
		nn := n.Next()
		if nn == nil {
			return n
		}
		nn = n
	}
}
