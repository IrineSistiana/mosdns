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
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"time"
)

const (
	PluginType = "cache"

	maxTTL uint32 = 3600 * 24 * 7 // one week
)

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(preset(handler.NewBP("_default_cache", PluginType), &Args{}), true)
}

var _ handler.ESExecutablePlugin = (*cachePlugin)(nil)
var _ handler.ContextPlugin = (*cachePlugin)(nil)

type Args struct {
	Size            int    `yaml:"size"`
	CleanerInterval int    `yaml:"cleaner_interval"`
	Redis           string `yaml:"redis"`
}

type cachePlugin struct {
	*handler.BP
	args *Args

	c cache
}

type cache interface {
	get(ctx context.Context, key string) (v []byte, ttl time.Duration, ok bool, err error)
	store(ctx context.Context, key string, v []byte, ttl time.Duration) (err error)
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newCachePlugin(bp, args.(*Args))
}

func newCachePlugin(bp *handler.BP, args *Args) (*cachePlugin, error) {
	var c cache
	var err error
	if len(args.Redis) != 0 {
		c, err = newRedisCache(args.Redis)
		if err != nil {
			return nil, err
		}
	} else {
		c = newMemCache(args.Size, time.Duration(args.CleanerInterval)*time.Second)
	}
	return &cachePlugin{
		BP:   bp,
		args: args,
		c:    c,
	}, nil
}

// ExecES searches the cache. If cache hits, earlyStop will be true.
// It never returns an err. Because a cache fault should not terminate the query process.
func (c *cachePlugin) ExecES(ctx context.Context, qCtx *handler.Context) (earlyStop bool, err error) {
	key, cacheHit := c.searchAndReply(ctx, qCtx)
	if cacheHit {
		return true, nil
	}

	if len(key) != 0 {
		de := newDeferExecutable(key, c)
		qCtx.DeferExec(de)
	}

	return false, nil
}

const (
	saltUDP uint16 = iota
	saltTCP
)

func (c *cachePlugin) searchAndReply(ctx context.Context, qCtx *handler.Context) (key string, cacheHit bool) {
	q := qCtx.Q()
	var salt uint16
	if qCtx.IsTCPClient() {
		salt = saltTCP
	} else {
		salt = saltUDP
	}
	key, err := utils.GetMsgKey(q, salt)
	if err != nil {
		c.L().Warn("unable to get msg key, skip it", qCtx.InfoField(), zap.Error(err))
		return "", false
	}
	v, ttl, _, err := c.c.get(ctx, key)
	if err != nil {
		c.L().Warn("unable to access cache, skip it", qCtx.InfoField(), zap.Error(err))
		return key, false
	}

	if len(v) != 0 { // if cache hit
		r := new(dns.Msg)
		if err := r.Unpack(v); err != nil {
			c.L().Warn("failed to unpack cached data", qCtx.InfoField(), zap.Error(err))
			return key, false
		}

		c.L().Debug("cache hit", qCtx.InfoField())
		r.Id = q.Id
		utils.SetTTL(r, uint32(ttl/time.Second))
		qCtx.SetResponse(r, handler.ContextStatusResponded)
		return key, true
	}
	return key, false
}

type deferExecutable struct {
	key string
	p   *cachePlugin
}

func newDeferExecutable(key string, p *cachePlugin) *deferExecutable {
	return &deferExecutable{key: key, p: p}
}

// Exec caches the response.
// It never returns an err. Because a cache fault should not terminate the query process.
func (d *deferExecutable) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	if err := d.exec(ctx, qCtx); err != nil {
		d.p.L().Warn("failed to cache the data", qCtx.InfoField(), zap.Error(err))
	}
	return nil
}

func (d *deferExecutable) exec(ctx context.Context, qCtx *handler.Context) (err error) {
	r := qCtx.R()
	if r != nil && r.Rcode == dns.RcodeSuccess && len(r.Answer) != 0 {
		ttl := utils.GetMinimalTTL(r)
		if ttl > maxTTL {
			ttl = maxTTL
		}
		buf := make([]byte, r.Len())
		v, err := r.PackBuffer(buf)
		if err != nil {
			return err
		}
		return d.p.c.store(ctx, d.key, v, time.Duration(ttl)*time.Second)
	}
	return nil
}

func (c *cachePlugin) Connect(ctx context.Context, qCtx *handler.Context, pipeCtx *handler.PipeContext) (err error) {
	key, cacheHit := c.searchAndReply(ctx, qCtx)
	if cacheHit {
		return nil
	}

	err = pipeCtx.ExecNextPlugin(ctx, qCtx)
	if err != nil {
		return err
	}

	if len(key) != 0 {
		_ = newDeferExecutable(key, c).Exec(ctx, qCtx)
	}

	return nil
}

func preset(bp *handler.BP, args *Args) *cachePlugin {
	p, err := newCachePlugin(bp, args)
	if err != nil {
		panic(fmt.Sprintf("cache: preset plugin: %s", err))
	}
	return p
}
