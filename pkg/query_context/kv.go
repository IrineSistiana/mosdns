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

package query_context

import "sync/atomic"

var kId atomic.Uint32

// RegKey returns a unique uint32 for the key used in
// Context.StoreValue, Context.GetValue.
// It should only be called during initialization.
func RegKey() uint32 {
	i := kId.Add(1)
	if i == 0 {
		panic("key id overflowed")
	}
	return i
}
