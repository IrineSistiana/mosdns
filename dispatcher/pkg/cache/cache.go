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
	"github.com/miekg/dns"
	"io"
	"time"
)

// Backend represents a DNS cache backend.
type Backend interface {
	// Get retrieves v from Backend. The returned v is a deepcopy of the original msg
	// if the key is stored in the cache. Otherwise, v is nil.
	// If allowExpired, v might be expired as long as the key is in the cache.
	// Note: The caller should change the TTLs and id of v.
	Get(ctx context.Context, key string, allowExpired bool) (v *dns.Msg, storedTime, expirationTime time.Time, err error)

	// Store stores a deepcopy of v into Backend. v cannot be nil.
	// If expirationTime is already passed, Store is a noop.
	Store(ctx context.Context, key string, v *dns.Msg, storedTime, expirationTime time.Time) (err error)

	// Closer closes the cache backend.
	io.Closer
}
