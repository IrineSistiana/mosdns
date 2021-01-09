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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"time"
)

const (
	PluginType = "cache"

	maxTTL uint32 = 3600
)

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(newCachePlugin(handler.NewBP("_default_cache", PluginType), &Args{}), true)
}

var _ handler.ContextPlugin = (*cachePlugin)(nil)

type Args struct {
	Size            int `yaml:"size"`
	CleanerInterval int `yaml:"cleaner_interval"`
}

type cachePlugin struct {
	*handler.BP

	c *cache
}

func (c *cachePlugin) Connect(ctx context.Context, qCtx *handler.Context, pipeCtx *handler.PipeContext) (err error) {
	return c.connect(ctx, qCtx, pipeCtx)
}

func (c *cachePlugin) connect(ctx context.Context, qCtx *handler.Context, pipeCtx *handler.PipeContext) (err error) {
	cacheable := true
	key, err := utils.GetMsgKey(qCtx.Q())
	if err != nil {
		c.L().Warn("unable to get msg key, skip the cache", qCtx.InfoField(), zap.Error(err))
		cacheable = false
	} else {
		if r, ttl := c.c.get(key); r != nil { // if cache hit
			c.L().Debug("cache hit", qCtx.InfoField())
			r.Id = qCtx.Q().Id
			setTTL(r, uint32(ttl/time.Second))
			qCtx.SetResponse(r, handler.ContextStatusResponded)
			return nil
		}
	}

	err = pipeCtx.ExecNextPlugin(ctx, qCtx)
	if err != nil {
		return err
	}

	if cacheable && qCtx.R() != nil && qCtx.R().Rcode == dns.RcodeSuccess && len(qCtx.R().Answer) != 0 {
		ttl := getMinimalTTL(qCtx.R())
		if ttl > maxTTL {
			ttl = maxTTL
		}
		c.c.add(key, ttl, qCtx.R())
	}

	return nil
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newCachePlugin(bp, args.(*Args)), nil
}

func newCachePlugin(bp *handler.BP, args *Args) *cachePlugin {
	return &cachePlugin{
		BP: bp,
		c:  newCache(args.Size, time.Duration(args.CleanerInterval)*time.Second),
	}
}
