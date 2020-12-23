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
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"time"
)

const (
	PluginType = "cache"

	maxTTL uint32 = 3600
)

func init() {
	handler.RegInitFunc(PluginType, Init)
}

var _ handler.PipelinePlugin = (*cachePipeLine)(nil)

type Args struct {
	Size            int  `yaml:"size"`
	CleanerInterval int  `yaml:"cleaner_interval"`
	CacheECS        bool `yaml:"cache_ecs"`
}

type cachePipeLine struct {
	tag string
	c   *cache
}

func (c *cachePipeLine) Tag() string {
	return c.tag
}

func (c *cachePipeLine) Type() string {
	return PluginType
}

func (c *cachePipeLine) Connect(ctx context.Context, qCtx *handler.Context, pipeCtx *handler.PipeContext) (err error) {
	if qCtx == nil || qCtx.Q == nil || pipeCtx == nil {
		return nil
	}

	cacheable := len(qCtx.Q.Question) == 1
	var key string
	if cacheable {
		key, err = utils.GetMsgKey(qCtx.Q)
		if err != nil {
			return err
		}

		if r := c.c.get(key); r != nil { // if cache hit
			r.Id = qCtx.Q.Id
			qCtx.R = r
			return nil
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

	return &cachePipeLine{
		tag: tag,
		c:   newCache(args.Size, time.Duration(args.CleanerInterval)*time.Second),
	}, nil
}
