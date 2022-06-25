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

package cache

import (
	"io"
	"time"
)

// Backend represents a cache backend.
// The Backend does not raise errors cause a cache error is not a
// fatal error to a dns query. The caller usually does not care too
// much about the cache error. Implements should handle errors themselves.
// Cache Backend is expected to be very fast. All operations should be
// done (or returned) in a short time. e.g. 50 ms.
type Backend interface {
	// Get retrieves v from Backend. The returned v may be the original value. The caller should
	// not modify it.
	Get(key string) (v []byte, storedTime, expirationTime time.Time)

	// Store stores a copy of v into Backend. v cannot be nil.
	// If expirationTime is already passed, Store is a noop.
	Store(key string, v []byte, storedTime, expirationTime time.Time)

	Len() int

	// Closer closes the cache backend. Get and Store should become noop calls.
	io.Closer
}
