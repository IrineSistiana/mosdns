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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
	"io"
	"time"
)

// DnsCache represents a DNS cache backend.
type DnsCache interface {
	// Get retrieves v from DnsCache. The returned v is a deepcopy of the original msg
	// that stored in the cache. The TTLs of v has already been modified properly.
	// The only thing that call should modify is msg's id.
	Get(ctx context.Context, key string) (v *dns.Msg, err error)
	// Store stores the v into DnsCache. It stores the deepcopy of v.
	Store(ctx context.Context, key string, v *dns.Msg, ttl time.Duration) (err error)

	// Closer closes the cache backend.
	io.Closer
}

// DeferCacheStore implements handler.Executable.
type DeferCacheStore struct {
	key     string
	backend DnsCache
}

func NewDeferStore(key string, backend DnsCache) *DeferCacheStore {
	return &DeferCacheStore{key: key, backend: backend}
}

// Exec caches the response.
// It never returns an err, because a cache fault should not terminate the query process.
func (d *DeferCacheStore) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	r := qCtx.R()
	if r != nil && r.Rcode == dns.RcodeSuccess && r.Truncated == false && len(r.Answer) != 0 {
		return d.backend.Store(ctx, d.key, r, time.Duration(dnsutils.GetMinimalTTL(r))*time.Second)
	}
	return nil
}
