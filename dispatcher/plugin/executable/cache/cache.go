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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/cache"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/cache/mem_cache"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/cache/redis_cache"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"log"
	"net"
	"os"
	"strings"
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
	Log               Log    `yaml:"log"`
	Size              int    `yaml:"size"`
	Redis             string `yaml:"redis"`
	Simple_cache      bool   `yaml:"simple_cache"`
	LazyCacheTTL      int    `yaml:"lazy_cache_ttl"`
	LazyCacheReplyTTL int    `yaml:"lazy_cache_reply_ttl"`
	CacheEverything   bool   `yaml:"cache_everything"`
}

type Log struct {
	File   string `yaml:"file"`
	Path   string `yaml:"path"`
	Size   int64  `yaml:"size"`
	Rotate int    `yaml:"rotate"`
}

type cachePlugin struct {
	*handler.BP
	args *Args
	ch   chan map[string]string

	backend      cache.Backend
	lazyUpdateSF singleflight.Group
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newCachePlugin(bp, args.(*Args))
}

func newCachePlugin(bp *handler.BP, args *Args) (*cachePlugin, error) {
	var c cache.Backend
	var err error
	ch := make(chan map[string]string, 1000)
	if args.Log.Path != "" {
		go golog(ch, args.Log.Path, args.Log.Size, args.Log.Rotate)
	}
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
		ch:      ch,
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
	var ClientIP net.IP
	if ecs := dnsutils.GetMsgECS(q); ecs != nil {
		ClientIP = ecs.Address
		dnsutils.RemoveMsgECS(q)
	} else if meta := qCtx.ReqMeta(); meta != nil {
		ClientIP = meta.ClientIP
	}

	if c.skip(q) {
		c.L().Debug("skipped", qCtx.InfoField())
		c.Sendlog(qCtx, ClientIP, "SKIP")
		return handler.ExecChainNode(ctx, qCtx, next)
	}

	var key string
	var err error
	if c.args.Simple_cache && len(q.Question) == 1 {
		key = q.Question[0].String() + c.Tag()
	} else {
		key, err = utils.GetMsgKey(q, 0)
		if err != nil {
			return fmt.Errorf("failed to get msg key, %w", err)
		}
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
			c.Sendlog(qCtx, ClientIP, "HIT")
			dnsutils.SubtractTTL(r, uint32(time.Since(storedTime).Seconds()))
			qCtx.SetResponse(r, handler.ContextStatusResponded)
			return nil
		}

		// expired but lazy update enabled
		if c.args.LazyCacheTTL > 0 {
			c.L().Debug("expired cache hit", qCtx.InfoField())
			c.Sendlog(qCtx, ClientIP, "EXP-HIT")
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
	c.Sendlog(qCtx, ClientIP, "MISS")
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

func (c *cachePlugin) Sendlog(qCtx *handler.Context, ClientIP net.IP, state string) {
	if c.args.Log.Path != "" {
		q := qCtx.Q()
		msg := make(map[string]string)
		msg[c.Tag()] = ClientIP.String() + " " + q.Question[0].Name + " " + dnsutils.QclassToString(q.Question[0].Qclass) + " " + dnsutils.QtypeToString(q.Question[0].Qtype) + " " + state
		c.ch <- msg
	}
}

func golog(ch chan map[string]string, Path string, Size int64, Rotate int) {
	if Path == "" {
		return
	}
	if !strings.HasSuffix(Path, "/") {
		Path = Path + "/"
	}
	if Size == 0 {
		Size = 2
	}
	Size = Size * 1024 * 1024
	if Rotate == 0 {
		Rotate = 5
	}
	logger_map := make(map[string]*log.Logger)
	logfile_map := make(map[string]*os.File)
	var logfile *os.File
	var logger *log.Logger
	for msg := range ch {
		for tag := range msg {
			File := Path + tag + ".log"
			if _, ok := logger_map[tag]; ok {
				logfile, _ = logfile_map[tag]
				logger, _ = logger_map[tag]
			} else {
				logger, logfile = NewLogger(File)
				logger_map[tag] = logger
				logfile_map[tag] = logfile
			}
			logger.Println(msg[tag])
			stat, err := logfile.Stat()
			if err == nil {
				if stat.Size() > Size {
					logfile.Close()
					doRotate(File, Rotate)
					logger, logfile = NewLogger(File)
					logger_map[tag] = logger
					logfile_map[tag] = logfile
				}
			}
		}
	}
	logfile.Close()
}

//https://www.cnblogs.com/mikezhang/p/golanglogrotate20170614.html
func doRotate(FileName string, Rotate int) {
	for j := Rotate; j >= 1; j-- {
		curFileName := fmt.Sprintf("%s_%d", FileName, j)
		k := j - 1
		preFileName := fmt.Sprintf("%s_%d", FileName, k)

		if k == 0 {
			preFileName = fmt.Sprintf("%s", FileName)
		}
		_, err := os.Stat(curFileName)
		if err == nil {
			os.Remove(curFileName)
			fmt.Println("remove : ", curFileName)
		}
		_, err = os.Stat(preFileName)
		if err == nil {
			fmt.Println("rename : ", preFileName, " => ", curFileName)
			err = os.Rename(preFileName, curFileName)
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func NewLogger(FileName string) (*log.Logger, *os.File) {
	var logger *log.Logger
	logFile, err := os.OpenFile(FileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("open log file error!", FileName)
		os.Exit(0)
	} else {
		logger = log.New(logFile, "", log.Ldate|log.Ltime)
	}
	return logger, logFile
}
