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
	"github.com/go-redis/redis/v8"
	"time"
)

type redisCache struct {
	client *redis.Client
}

func newRedisCache(url string) (*redisCache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}

	c := redis.NewClient(opt)
	return &redisCache{client: c}, nil
}

func (r *redisCache) get(ctx context.Context, key string) (v []byte, ttl time.Duration, ok bool, err error) {
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
	return b, ttl, true, nil
}

func (r *redisCache) store(ctx context.Context, key string, v []byte, ttl time.Duration) (err error) {
	return r.client.Set(ctx, key, v, ttl).Err()
}

// nothing to reuse
func (r *redisCache) release(v []byte) {}
