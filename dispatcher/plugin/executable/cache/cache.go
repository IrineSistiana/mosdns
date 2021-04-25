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
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/cache/mem_cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/cache/redis_cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"go.uber.org/zap"
	"time"
)

const (
	PluginType = "cache"
)

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(preset(handler.NewBP("_default_cache", PluginType), &Args{}), true)
}

var _ handler.ESExecutablePlugin = (*cachePlugin)(nil)

type Args struct {
	Size            int    `yaml:"size"`
	CleanerInterval int    `yaml:"cleaner_interval"`
	Redis           string `yaml:"redis"`
}

type cachePlugin struct {
	*handler.BP
	args *Args

	c cache.DnsCache
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newCachePlugin(bp, args.(*Args))
}

func newCachePlugin(bp *handler.BP, args *Args) (*cachePlugin, error) {
	var c cache.DnsCache
	var err error
	if len(args.Redis) != 0 {
		c, err = redis_cache.NewRedisCache(args.Redis)
		if err != nil {
			return nil, err
		}
	} else {
		if args.Size <= 1024 {
			args.Size = 1024
		}

		sizePerShard := args.Size / 32

		if args.CleanerInterval == 0 {
			args.CleanerInterval = 120
		}

		c = mem_cache.NewMemCache(32, sizePerShard, time.Duration(args.CleanerInterval)*time.Second)
	}
	return &cachePlugin{
		BP:   bp,
		args: args,
		c:    c,
	}, nil
}

// ExecES searches the cache. If cache hits, earlyStop will be true.
// It never returns an err, because a cache fault should not terminate the query process.
func (c *cachePlugin) ExecES(ctx context.Context, qCtx *handler.Context) (earlyStop bool, err error) {
	q := qCtx.Q()
	key, err := utils.GetMsgKey(q, 0)
	if err != nil {
		c.L().Warn("unable to get msg key", qCtx.InfoField(), zap.Error(err))
		return false, nil
	}

	// lookup in cache
	r, err := c.c.Get(ctx, key)
	if err != nil {
		c.L().Warn("unable to access cache", qCtx.InfoField(), zap.Error(err))
		return false, nil
	}

	// cache hit
	if r != nil {
		r.Id = q.Id
		c.L().Debug("cache hit", qCtx.InfoField())
		qCtx.SetResponse(r, handler.ContextStatusResponded)
		return true, nil
	}

	// cache miss
	qCtx.DeferExec(cache.NewDeferStore(key, c.c))
	return false, nil
}

func preset(bp *handler.BP, args *Args) *cachePlugin {
	p, err := newCachePlugin(bp, args)
	if err != nil {
		panic(fmt.Sprintf("cache: preset plugin: %s", err))
	}
	return p
}
