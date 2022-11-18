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

package sequence

// NewTestChainWalker returns a ChainWalker which ChainWalker.ExecNext
// will always call nextExec with a noop ChainWalker.
// Note: As the function name indicates, this is for tests only.
func NewTestChainWalker(nextExec RecursiveExecutable) ChainWalker {
	return ChainWalker{
		chain: []*chainNode{{
			re: nextExec,
		}},
	}
}
