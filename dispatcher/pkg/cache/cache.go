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

package cache

import (
	"context"
	"io"
	"time"
)

// Backend represents a cache backend.
type Backend interface {
	// Get retrieves v from Backend. The returned v may be the original value. The caller should
	// not modify it.
	Get(ctx context.Context, key string) (v []byte, storedTime, expirationTime time.Time, err error)

	// Store stores a copy of v into Backend. v cannot be nil.
	// If expirationTime is already passed, Store is a noop.
	Store(ctx context.Context, key string, v []byte, storedTime, expirationTime time.Time) error

	// Closer closes the cache backend.
	io.Closer
}
