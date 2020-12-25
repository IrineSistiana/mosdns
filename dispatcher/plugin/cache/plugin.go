//     Copyright (C) 2020, IrineSistiana
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
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	PluginType = "cache"

	maxTTL uint32 = 3600
)

func init() {
	handler.RegInitFunc(PluginType, Init)

	handler.MustRegPlugin(newCachePlugin("_default_cache", &Args{Size: 1024, CleanerInterval: 10}))
}

var _ handler.ContextPlugin = (*cachePlugin)(nil)

type Args struct {
	Size            int `yaml:"size"`
	CleanerInterval int `yaml:"cleaner_interval"`
}

type cachePlugin struct {
	tag    string
	logger *logrus.Entry
	c      *cache
}

func (c *cachePlugin) Tag() string {
	return c.tag
}

func (c *cachePlugin) Type() string {
	return PluginType
}

func (c *cachePlugin) Connect(ctx context.Context, qCtx *handler.Context, pipeCtx *handler.PipeContext) (err error) {
	err = c.connect(ctx, qCtx, pipeCtx)
	if err != nil {
		return handler.NewPluginError(c.tag, err)
	}
	return nil
}

func (c *cachePlugin) connect(ctx context.Context, qCtx *handler.Context, pipeCtx *handler.PipeContext) (err error) {
	if qCtx == nil || qCtx.Q == nil || pipeCtx == nil {
		return nil
	}

	cacheable := len(qCtx.Q.Question) == 1
	var key string
	if cacheable {
		key, err = utils.GetMsgKey(qCtx.Q)
		if err != nil {
			c.logger.Warnf("%v: unable to get msg key, skip the cache: %v", qCtx, err)
			cacheable = false
		} else {
			if r, ttl := c.c.get(key); r != nil { // if cache hit
				c.logger.Warnf("%v: cache hit", qCtx)
				r.Id = qCtx.Q.Id
				setTTL(r, uint32(ttl/time.Second))
				qCtx.R = r
				return nil
			}
		}
	}

	err = pipeCtx.ExecNextPlugin(ctx, qCtx)
	if err != nil {
		return err
	}

	if cacheable && qCtx.R != nil && qCtx.R.Rcode == dns.RcodeSuccess {
		ttl := getMinimalTTL(qCtx.R)
		if ttl > maxTTL {
			ttl = maxTTL
		}
		c.c.add(key, ttl, qCtx.R)
	}

	return nil
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	return newCachePlugin(tag, args), nil
}

func newCachePlugin(tag string, args *Args) *cachePlugin {
	return &cachePlugin{
		tag:    tag,
		logger: mlog.NewPluginLogger(tag),
		c:      newCache(args.Size, time.Duration(args.CleanerInterval)*time.Second),
	}
}
