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
	"sync"
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

	msgPool sync.Pool
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newCachePlugin(bp, args.(*Args))
}

func newCachePlugin(bp *coremain.BP, args *Args) (*cachePlugin, error) {
	var c cache.Backend
	if len(args.Redis)!= 0 {
		opt, err := redis.ParseURL(args.Redis)
		if err!= nil {
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
		if err!= nil {
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

		msgPool: sync.Pool{
			New: func() interface{} {
				return new(dns.Msg)
			},
		},
	}
	bp.GetMetricsReg().MustRegister(p.queryTotal, p.hitTotal, p.lazyHitTotal, p.size)
	return p, nil
}

func (c *cachePlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	c.queryTotal.Inc()
	q := qCtx.Q()

	msgKey, err := c.getMsgKey(q)
	if err!= nil {
		c.L().Error("get msg key", qCtx.InfoField(), zap.Error(err))
	}
	if len(msgKey) == 0 { // skip cache
		return executable_seq.ExecChainNode(ctx, qCtx, next)
	}

	cachedResp, lazyHit, err := c.lookupCache(msgKey)
	if err!= nil {
		c.L().Error("lookup cache", qCtx.InfoField(), zap.Error(err))
	}
	if lazyHit {
		c.lazyHitTotal.Inc()
		c.doLazyUpdate(msgKey, qCtx, next)
	}
	if cachedResp!= nil { // cache hit
		c.hitTotal.Inc()
		cachedResp.Id = q.Id // change msg id

		// Use the message pool to get a new message object
		msg := c.msgPool.Get().(*dns.Msg)
		defer c.msgPool.Put(msg)

		// Copy the cached response to the new message object
		msg.Copy(cachedResp)

		// Set the response in the query context
		qCtx.SetResponse(msg)

		if c.whenHit!= nil {
			return c.whenHit.Exec(ctx, qCtx, nil)
		}
		return nil
	}

	// cache miss, run the entry and try to store its response.
	c.L().Debug("cache miss", qCtx.InfoField())
	err = executable_seq.ExecChainNode(ctx, qCtx, next)
	r := qCtx.R()
	if r!= nil {
		if err := c.tryStoreMsg(msgKey, r); err!= nil {
			c.L().Error("cache store", qCtx.InfoField(), zap.Error
