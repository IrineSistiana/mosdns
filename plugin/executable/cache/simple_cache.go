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
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/cache"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"strconv"
)

type simpleCache struct {
	backend *cache.Cache[key, *item]
}

func (c *simpleCache) Close() error {
	return c.backend.Close()
}

func (c *simpleCache) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	q := qCtx.Q()
	msgKey := getMsgKey(q)
	if len(msgKey) == 0 {
		return next.ExecNext(ctx, qCtx)
	}

	cachedResp, _ := getRespFromCache(msgKey, c.backend, false, 0)
	if cachedResp != nil {
		cachedResp.Id = q.Id
		shuffleIP(cachedResp)
		qCtx.SetResponse(cachedResp)
	}
	err := next.ExecNext(ctx, qCtx)

	if r := qCtx.R(); r != nil && r != cachedResp {
		saveRespToCache(msgKey, r, c.backend, 0)
	}
	return err
}

// QuickSetup format: [size]
// default is 1024. If size is < 1024, 1024 will be used.
func quickSetupCache(_ sequence.BQ, s string) (any, error) {
	size := 0
	if len(s) > 0 {
		i, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("invalid size, %w", err)
		}
		size = i
	}
	return &simpleCache{backend: cache.New[key, *item](cache.Opts{Size: size})}, nil
}
