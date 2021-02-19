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

func (r *RedisCache) Get(ctx context.Context, key string) (v *dns.Msg, ttl time.Duration, ok bool, err error) {
	b, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	ttl, err = r.client.TTL(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}

	v = new(dns.Msg)
	if err := v.Unpack(b); err != nil {
		return nil, 0, false, err
	}
	return v, ttl, true, nil
}

func (r *RedisCache) Store(ctx context.Context, key string, v *dns.Msg, ttl time.Duration) (err error) {
	wireMsg, buf, err := pool.PackBuffer(v)
	if err != nil {
		return err
	}
	defer pool.ReleaseMsgBuf(buf)
	return r.client.Set(ctx, key, wireMsg, ttl).Err()
}

// Close closes the redis client.
func (r *RedisCache) Close() error {
	return r.client.Close()
}
