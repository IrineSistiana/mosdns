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
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache/mem_cache"
	"github.com/IrineSistiana/mosdns/v4/pkg/cache/redis_cache"
	"github.com/IrineSistiana/mosdns/v4/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/pool"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/go-redis/redis/v8"
	"github.com/golang/snappy"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"time"
)

const (
	PluginType = "cache"
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })

	coremain.RegNewPersetPluginFunc("_default_cache", func(bp *coremain.BP) (coremain.Plugin, error) {
		return newCachePlugin(bp, &Args{})
	})
}

const (
	defaultLazyUpdateTimeout = time.Second * 5
	defaultEmptyAnswerTTL    = time.Second * 300
)

var _ coremain.ExecutablePlugin = (*cachePlugin)(nil)

type Args struct {
	Size              int    `yaml:"size"`
	Redis             string `yaml:"redis"`
	RedisTimeout      int    `yaml:"redis_timeout"`
	LazyCacheTTL      int    `yaml:"lazy_cache_ttl"`
	LazyCacheReplyTTL int    `yaml:"lazy_cache_reply_ttl"`
	CacheEverything   bool   `yaml:"cache_everything"`
	CompressResp      bool   `yaml:"compress_resp"`
	WhenHit           string `yaml:"when_hit"`
}

type cachePlugin struct {
	*coremain.BP
	args *Args

	whenHit      executable_seq.Executable
	backend      cache.Backend
	lazyUpdateSF singleflight.Group

	queryTotal   prometheus.Counter
	hitTotal     prometheus.Counter
	lazyHitTotal prometheus.Counter
	size         prometheus.GaugeFunc
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newCachePlugin(bp, args.(*Args))
}

func newCachePlugin(bp *coremain.BP, args *Args) (*cachePlugin, error) {
	var c cache.Backend
	if len(args.Redis) != 0 {
		opt, err := redis.ParseURL(args.Redis)
		if err != nil {
			return nil, fmt.Errorf("invalid redis url, %w", err)
		}
		opt.MaxRetries = -1
		r := redis.NewClient(opt)
		rcOpts := redis_cache.RedisCacheOpts{
			Client:        r,
			ClientCloser:  r,
			ClientTimeout: time.Duration(args.RedisTimeout) * time.Millisecond,
			Logger:        bp.L(),
		}
		rc, err := redis_cache.NewRedisCache(rcOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to init redis cache, %w", err)
		}
		c = rc
	} else {
		c = mem_cache.NewMemCache(args.Size, 0)
	}

	if args.LazyCacheReplyTTL <= 0 {
		args.LazyCacheReplyTTL = 5
	}

	var whenHit executable_seq.Executable
	if tag := args.WhenHit; len(tag) > 0 {
		m := bp.M().GetExecutables()
		whenHit = m[tag]
		if whenHit == nil {
			return nil, fmt.Errorf("cannot find exectable %s", tag)
		}
	}

	p := &cachePlugin{
		BP:      bp,
		args:    args,
		whenHit: whenHit,
		backend: c,

		queryTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "query_total",
			Help: "The total number of processed queries",
		}),
		hitTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hit_total",
			Help: "The total number of queries that hit the cache",
		}),
		lazyHitTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "lazy_hit_total",
			Help: "The total number of queries that hit the expired cache",
		}),
		size: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "cache_size",
			Help: "Current cache size in records",
		}, func() float64 {
			return float64(c.Len())
		}),
	}
	bp.GetMetricsReg().MustRegister(p.queryTotal, p.hitTotal, p.lazyHitTotal, p.size)
	return p, nil
}

func (c *cachePlugin) skip(q *dns.Msg) bool {
	if c.args.CacheEverything {
		return false
	}
	// We only cache simple queries.
	return !(len(q.Question) == 1 && len(q.Answer)+len(q.Ns)+len(q.Extra) == 0)
}

func (c *cachePlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	c.queryTotal.Inc()
	q := qCtx.Q()
	if c.skip(q) {
		c.L().Debug("skipped", qCtx.InfoField())
		return executable_seq.ExecChainNode(ctx, qCtx, next)
	}

	msgKey, err := dnsutils.GetMsgKey(q, 0)
	if err != nil {
		return fmt.Errorf("failed to get msg key, %w", err)
	}

	cachedResp, lazyHit, err := c.lookupCache(msgKey)
	if err != nil {
		c.L().Error("cache lookup", zap.Error(err))
	}
	if lazyHit {
		c.lazyHitTotal.Inc()
		c.doLazyUpdate(msgKey, qCtx, next)
	}
	if cachedResp != nil { // cache hit
		c.hitTotal.Inc()
		cachedResp.Id = q.Id // change msg id
		c.L().Debug("cache hit", qCtx.InfoField())
		qCtx.SetResponse(cachedResp)
		if c.whenHit != nil {
			return c.whenHit.Exec(ctx, qCtx, nil)
		}
	}

	// cache miss, run the entry and try to store its response.
	c.L().Debug("cache miss", qCtx.InfoField())
	err = executable_seq.ExecChainNode(ctx, qCtx, next)
	r := qCtx.R()
	if r != nil {
		c.tryStoreMsg(msgKey, r)
	}
	return err
}

// lookupCache returns the cached response. The ttl of returned msg will be changed properly.
// Remember, caller must change the msg id.
func (c *cachePlugin) lookupCache(msgKey string) (r *dns.Msg, lazyHit bool, err error) {
	// lookup in cache
	v, storedTime, _ := c.backend.Get(msgKey)

	// cache hit
	if v != nil {
		if c.args.CompressResp {
			decodeLen, err := snappy.DecodedLen(v)
			if err != nil {
				return nil, false, fmt.Errorf("snappy decode err: %w", err)
			}
			if decodeLen > dns.MaxMsgSize {
				return nil, false, fmt.Errorf("invalid snappy data, not a dns msg, data len: %d", decodeLen)
			}
			decompressBuf := pool.GetBuf(decodeLen)
			defer decompressBuf.Release()
			v, err = snappy.Decode(decompressBuf.Bytes(), v)
			if err != nil {
				return nil, false, fmt.Errorf("snappy decode err: %w", err)
			}
		}
		r = new(dns.Msg)
		if err := r.Unpack(v); err != nil {
			return nil, false, fmt.Errorf("failed to unpack cached data, %w", err)
		}

		var msgTTL time.Duration
		if len(r.Answer) == 0 {
			msgTTL = defaultEmptyAnswerTTL
		} else {
			msgTTL = time.Duration(dnsutils.GetMinimalTTL(r)) * time.Second
		}

		// not expired
		if storedTime.Add(msgTTL).After(time.Now()) {
			dnsutils.SubtractTTL(r, uint32(time.Since(storedTime).Seconds()))
			return r, false, nil
		}

		// expired but lazy update enabled
		if c.args.LazyCacheTTL > 0 {
			// set the default ttl
			dnsutils.SetTTL(r, uint32(c.args.LazyCacheReplyTTL))
			return r, true, nil
		}
	}

	// cache miss
	return nil, false, nil
}

func (c *cachePlugin) doLazyUpdate(msgKey string, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) {
	// start a goroutine to update cache in the background
	lazyQCtx := qCtx.Copy()
	lazyUpdateFunc := func() (interface{}, error) {
		c.L().Debug("start lazy cache update", lazyQCtx.InfoField())
		defer c.lazyUpdateSF.Forget(msgKey)
		lazyCtx, cancel := context.WithTimeout(context.Background(), defaultLazyUpdateTimeout)
		defer cancel()

		err := executable_seq.ExecChainNode(lazyCtx, lazyQCtx, next)
		if err != nil {
			c.L().Warn("failed to update lazy cache", lazyQCtx.InfoField(), zap.Error(err))
		}

		r := lazyQCtx.R()
		if r != nil {
			c.tryStoreMsg(msgKey, r)
		}
		c.L().Debug("lazy cache updated", lazyQCtx.InfoField())
		return nil, nil
	}
	c.lazyUpdateSF.DoChan(msgKey, lazyUpdateFunc) // DoChan won't block this goroutine
}

// tryStoreMsg tries to store r to cache. If r should be cached.
func (c *cachePlugin) tryStoreMsg(key string, r *dns.Msg) {
	if r.Rcode != dns.RcodeSuccess || r.Truncated != false {
		return
	}

	v, err := r.Pack()
	if err != nil {
		c.L().Warn("failed to pack msg", zap.Error(err))
		return
	}

	now := time.Now()
	var expirationTime time.Time
	if c.args.LazyCacheTTL > 0 {
		expirationTime = now.Add(time.Duration(c.args.LazyCacheTTL) * time.Second)
	} else {
		minTTL := dnsutils.GetMinimalTTL(r)
		if minTTL == 0 {
			return
		}
		expirationTime = now.Add(time.Duration(minTTL) * time.Second)
	}
	if c.args.CompressResp {
		compressBuf := pool.GetBuf(snappy.MaxEncodedLen(len(v)))
		v = snappy.Encode(compressBuf.Bytes(), v)
		defer compressBuf.Release()
	}
	c.backend.Store(key, v, now, expirationTime)
}

func (c *cachePlugin) Shutdown() error {
	return c.backend.Close()
}
