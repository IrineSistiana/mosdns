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

package reverselookup

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache/mem_cache"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache/redis_cache"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"time"
)

type store struct {
	cache cache.Backend
}

type storeOpts struct {
	size   int
	redis  string
	logger *zap.Logger
}

func newStore(opts storeOpts) (*store, error) {
	if u := opts.redis; len(u) > 0 {
		redisOpts, err := redis.ParseURL(u)
		if err != nil {
			return nil, fmt.Errorf("failed to parse redis url, %w", err)
		}
		return &store{cache: &redis_cache.RedisCache{
			Client: redis.NewClient(redisOpts),
			Logger: opts.logger,
		}}, nil
	}
	return &store{
		cache: mem_cache.NewMemCache(opts.size, 0),
	}, nil
}

func (s *store) save(ip string, fqdn string, ttl time.Duration) {
	if len(fqdn) == 0 || len(ip) == 0 || ttl <= 0 {
		return
	}
	now := time.Now()
	s.cache.Store(ip, []byte(fqdn), now, now.Add(ttl))
}

func (s *store) lookup(ip string) string {
	v, _, _ := s.cache.Get(ip)
	if v == nil {
		return ""
	}
	return string(v)
}

func (s *store) close() {
	s.cache.Close()
}
