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

package redis_cache

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/pool"
	"github.com/go-redis/redis/v8"
	"github.com/miekg/dns"
	"time"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(url string) (*RedisCache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}

	c := redis.NewClient(opt)
	return &RedisCache{client: c}, nil
}

func (r *RedisCache) Get(ctx context.Context, key string) (v *dns.Msg, err error) {
	b, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	storedTime, m, err := unpackRedisValue(b)
	if err != nil {
		return nil, err
	}

	dnsutils.SubtractTTL(m, uint32(time.Since(storedTime)/time.Second))
	return m, nil
}

func (r *RedisCache) Store(ctx context.Context, key string, v *dns.Msg, ttl time.Duration) (err error) {
	if ttl <= 0 {
		return nil
	}

	data, err := packRedisValue(time.Now(), v)
	if err != nil {
		return err
	}
	defer pool.ReleaseBuf(data)

	return r.client.Set(ctx, key, data, ttl).Err()
}

// Close closes the redis client.
func (r *RedisCache) Close() error {
	return r.client.Close()
}

// packRedisValue packs storedTime and msg m into one byte slice.
// The returned []byte can be released pool.ReleaseBuf().
func packRedisValue(storedTime time.Time, m *dns.Msg) ([]byte, error) {
	wireMsg, bm, err := pool.PackBuffer(m)
	if err != nil {
		return nil, err
	}
	defer pool.ReleaseBuf(bm)

	v := pool.GetBuf(8 + len(wireMsg))
	binary.BigEndian.PutUint64(v[:8], uint64(storedTime.Unix()))
	copy(v[8:], wireMsg)
	return v, nil
}

func unpackRedisValue(b []byte) (storedTime time.Time, m *dns.Msg, err error) {
	if len(b) < 8 {
		return time.Time{}, nil, errors.New("b is too short")
	}
	storedTime = time.Unix(int64(binary.BigEndian.Uint64(b[:8])), 0)

	m = new(dns.Msg)
	err = m.Unpack(b[8:])
	if err != nil {
		return time.Time{}, nil, err
	}
	return storedTime, m, nil
}
