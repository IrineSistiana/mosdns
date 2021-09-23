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
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"time"
)

const (
	PluginType               = "cache"
	defaultLazyUpdateTimeout = time.Second * 5
)

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ESExecutablePlugin = (*cachePlugin)(nil)

type Args struct {
	Size               int      `yaml:"size"`
	CleanerInterval    int      `yaml:"cleaner_interval"`
	Redis              string   `yaml:"redis"`
	LazyUpdateSequence []string `yaml:"lazy_update_sequence"`
	LazyCacheTTL       uint32   `yaml:"lazy_cache_ttl"`
}

type cachePlugin struct {
	*handler.BP
	args *Args

	backend cache.Backend

	lazyUpdateExecutableCmd executable_seq.ExecutableNode
	lazyUpdateSF            singleflight.Group
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newCachePlugin(bp, args.(*Args))
}

func newCachePlugin(bp *handler.BP, args *Args) (*cachePlugin, error) {
	p := &cachePlugin{
		BP:   bp,
		args: args,
	}

	if len(args.Redis) != 0 {
		backend, err := redis_cache.NewRedisCache(args.Redis)
		if err != nil {
			return nil, err
		}
		p.backend = backend
	} else {
		if args.Size <= 1024 {
			args.Size = 1024
		}

		sizePerShard := args.Size / 32

		if args.CleanerInterval == 0 {
			args.CleanerInterval = 120
		}

		p.backend = mem_cache.NewMemCache(32, sizePerShard, time.Duration(args.CleanerInterval)*time.Second)
	}

	if len(args.LazyUpdateSequence) != 0 {
		luec, err := executable_seq.ParseExecutableNode(args.LazyUpdateSequence)
		if err != nil {
			return nil, err
		}
		p.lazyUpdateExecutableCmd = luec
	}

	return p, nil
}

// ExecES searches the cache. If cache hits, earlyStop will be true.
// It never returns an error, because a cache fault should not terminate the query process.
func (c *cachePlugin) ExecES(ctx context.Context, qCtx *handler.Context) (bool, error) {
	lazyUpdate := c.lazyUpdateExecutableCmd != nil

	q := qCtx.Q()
	key, err := utils.GetMsgKey(q, 0)
	if err != nil {
		c.L().Warn("unable to get msg key", qCtx.InfoField(), zap.Error(err))
		return false, nil
	}

	if r := qCtx.R(); r != nil { // already has a response. Store it and return.
		if cacheAble(r) {
			err := c.storeMsg(ctx, key, r)
			if err != nil {
				return false, fmt.Errorf("failed to store msg, %w", err)
			}
		}
		return false, nil
	}

	// lookup in cache
	r, storedTime, expirationTime, err := c.backend.Get(ctx, key, lazyUpdate)
	if err != nil {
		return false, err
	}
	if err != nil {
		c.L().Warn("unable to access cache", qCtx.InfoField(), zap.Error(err))
		return false, nil
	}

	// cache hit
	if r != nil {
		r.Id = q.Id // change msg id

		if lazyUpdate && expirationTime.Before(time.Now()) { // lazy update enabled and expired cache hit

			// prepare a response with 1 ttl
			dnsutils.SetTTL(r, 1)
			c.L().Debug("expired cache hit", qCtx.InfoField())
			qCtx.SetResponse(r, handler.ContextStatusResponded)

			// start a goroutine to update cache
			lazyUpdateDdl, ok := ctx.Deadline()
			if !ok {
				lazyUpdateDdl = time.Now().Add(defaultLazyUpdateTimeout)
			}
			lazyQCtx := qCtx.CopyNoR()

			lazyUpdateFunc := func() (interface{}, error) {
				defer c.lazyUpdateSF.Forget(key)
				lazyCtx, cancel := context.WithDeadline(context.Background(), lazyUpdateDdl)
				defer cancel()

				_, err := c.lazyUpdateExecutableCmd.Exec(lazyCtx, lazyQCtx, c.L())
				if err != nil {
					c.L().Warn("lazy update sequence err", lazyQCtx.InfoField(), zap.Error(err))
				}

				r := qCtx.R()
				if r != nil && cacheAble(r) {
					err := c.storeMsg(ctx, key, r)
					if err != nil {
						c.L().Warn("lazy update err", lazyQCtx.InfoField(), zap.Error(err))
					}
				}
				return nil, nil
			}

			c.lazyUpdateSF.DoChan(key, lazyUpdateFunc) // DoChan won't block this goroutine
			return true, nil
		}

		dnsutils.SubtractTTL(r, uint32(time.Since(storedTime).Seconds()))
		c.L().Debug("cache hit", qCtx.InfoField())
		qCtx.SetResponse(r, handler.ContextStatusResponded)
		return true, nil
	}

	// cache miss, do nothing
	return false, nil
}

func cacheAble(r *dns.Msg) bool {
	return r.Rcode == dns.RcodeSuccess && r.Truncated == false && len(r.Answer) != 0
}

func (c *cachePlugin) storeMsg(ctx context.Context, key string, r *dns.Msg) error {
	now := time.Now()
	var expirationTime time.Time
	if c.lazyUpdateExecutableCmd != nil {
		expirationTime = now.Add(time.Duration(c.args.LazyCacheTTL) * time.Second)
	} else {
		expirationTime = now.Add(time.Duration(dnsutils.GetMinimalTTL(r)) * time.Second)
	}
	return c.backend.Store(ctx, key, r, now, expirationTime)
}
