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
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/cache"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/cache/mem_cache"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/cache/redis_cache"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"time"
)

const (
	PluginType = "cache"
)

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(preset(handler.NewBP("_default_cache", PluginType), &Args{}))
}

const (
	defaultLazyUpdateTimeout = time.Second * 5
	defaultEmptyAnswerTTL    = time.Second * 300
)

var _ handler.ExecutablePlugin = (*cachePlugin)(nil)

type Args struct {
	Size              int    `yaml:"size"`
	Redis             string `yaml:"redis"`
	LazyCacheTTL      int    `yaml:"lazy_cache_ttl"`
	LazyCacheReplyTTL int    `yaml:"lazy_cache_reply_ttl"`
	CacheEverything   bool   `yaml:"cache_everything"`
}

type cachePlugin struct {
	*handler.BP
	args *Args

	backend      cache.Backend
	lazyUpdateSF singleflight.Group
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newCachePlugin(bp, args.(*Args))
}

func newCachePlugin(bp *handler.BP, args *Args) (*cachePlugin, error) {
	var c cache.Backend
	var err error
	if len(args.Redis) != 0 {
		c, err = redis_cache.NewRedisCache(args.Redis)
		if err != nil {
			return nil, err
		}
	} else {
		c = mem_cache.NewMemCache(args.Size, 0)
	}

	if args.LazyCacheReplyTTL <= 0 {
		args.LazyCacheReplyTTL = 30
	}

	return &cachePlugin{
		BP:      bp,
		args:    args,
		backend: c,
	}, nil
}

func (c *cachePlugin) skip(q *dns.Msg) bool {
	if c.args.CacheEverything {
		return false
	}
	// We only cache simple queries.
	return !(len(q.Question) == 1 && len(q.Answer)+len(q.Ns)+len(q.Extra) == 0)
}

func (c *cachePlugin) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	q := qCtx.Q()
	if c.skip(q) {
		c.L().Debug("skipped", qCtx.InfoField())
		return handler.ExecChainNode(ctx, qCtx, next)
	}

	key, err := utils.GetMsgKey(q, 0)
	if err != nil {
		return fmt.Errorf("failed to get msg key, %w", err)
	}

	// lookup in cache
	v, storedTime, _, err := c.backend.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("unable to access cache, %w", err)
	}

	// cache hit
	if v != nil {
		r := new(dns.Msg)
		if err := r.Unpack(v); err != nil {
			return fmt.Errorf("failed to unpack cached data, %w", err)
		}
		// change msg id to query
		r.Id = q.Id

		var msgTTL time.Duration
		if len(r.Answer) == 0 {
			msgTTL = defaultEmptyAnswerTTL
		} else {
			msgTTL = time.Duration(dnsutils.GetMinimalTTL(r)) * time.Second
		}
		if storedTime.Add(msgTTL).After(time.Now()) { // not expired
			c.L().Debug("cache hit", qCtx.InfoField())
			dnsutils.SubtractTTL(r, uint32(time.Since(storedTime).Seconds()))
			qCtx.SetResponse(r, handler.ContextStatusResponded)
			return nil
		}

		// expired but lazy update enabled
		if c.args.LazyCacheTTL > 0 {
			c.L().Debug("expired cache hit", qCtx.InfoField())
			// prepare a response with 1 ttl
			dnsutils.SetTTL(r, uint32(c.args.LazyCacheReplyTTL))
			qCtx.SetResponse(r, handler.ContextStatusResponded)

			// start a goroutine to update cache
			lazyUpdateDdl, ok := ctx.Deadline()
			if !ok {
				lazyUpdateDdl = time.Now().Add(defaultLazyUpdateTimeout)
			}
			lazyQCtx := qCtx.Copy()
			lazyUpdateFunc := func() (interface{}, error) {
				c.L().Debug("start lazy cache update", lazyQCtx.InfoField(), zap.Error(err))
				defer c.lazyUpdateSF.Forget(key)
				lazyCtx, cancel := context.WithDeadline(context.Background(), lazyUpdateDdl)
				defer cancel()

				err := handler.ExecChainNode(lazyCtx, lazyQCtx, next)
				if err != nil {
					c.L().Warn("failed to update lazy cache", lazyQCtx.InfoField(), zap.Error(err))
				}

				r := lazyQCtx.R()
				if r != nil {
					err := c.tryStoreMsg(lazyCtx, key, r)
					if err != nil {
						c.L().Warn("failed to store lazy cache", lazyQCtx.InfoField(), zap.Error(err))
					}
				}
				c.L().Debug("lazy cache updated", lazyQCtx.InfoField())
				return nil, nil
			}
			c.lazyUpdateSF.DoChan(key, lazyUpdateFunc) // DoChan won't block this goroutine
			return nil
		}
	}

	// cache miss, run the entry and try to store its response.
	c.L().Debug("cache miss", qCtx.InfoField())
	err = handler.ExecChainNode(ctx, qCtx, next)
	r := qCtx.R()
	if r != nil {
		err := c.tryStoreMsg(ctx, key, r)
		if err != nil {
			c.L().Warn("failed to store cache", qCtx.InfoField(), zap.Error(err))
		}
	}
	return err
}

// tryStoreMsg tries to store r to cache. If r should be cached.
func (c *cachePlugin) tryStoreMsg(ctx context.Context, key string, r *dns.Msg) error {
	if r.Rcode != dns.RcodeSuccess || r.Truncated != false {
		return nil
	}

	v, err := r.Pack()
	if err != nil {
		return fmt.Errorf("failed to pack msg, %w", err)
	}

	now := time.Now()
	var expirationTime time.Time
	if c.args.LazyCacheTTL > 0 {
		expirationTime = now.Add(time.Duration(c.args.LazyCacheTTL) * time.Second)
	} else {
		minTTL := dnsutils.GetMinimalTTL(r)
		if minTTL == 0 {
			return nil
		}
		expirationTime = now.Add(time.Duration(minTTL) * time.Second)
	}
	return c.backend.Store(ctx, key, v, now, expirationTime)
}

func (c *cachePlugin) Shutdown() error {
	return c.backend.Close()
}

func preset(bp *handler.BP, args *Args) *cachePlugin {
	p, err := newCachePlugin(bp, args)
	if err != nil {
		panic(fmt.Sprintf("cache: preset plugin: %s", err))
	}
	return p
}
