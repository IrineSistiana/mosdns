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

package fastforward

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/safe_close"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strings"
	"time"
)

const PluginType = "forward"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
	sequence.MustRegExecQuickSetup(PluginType, quickSetup)
}

const (
	maxConcurrentQueries = 3
	queryTimeout         = time.Second * 5
)

type Args struct {
	Upstreams  []UpstreamConfig `yaml:"upstreams"`
	Concurrent int              `yaml:"concurrent"`

	// Global options.
	Socks5       string `yaml:"socks5"`
	SoMark       int    `yaml:"so_mark"`
	BindToDevice string `yaml:"bind_to_device"`
	Bootstrap    string `yaml:"bootstrap"`
}

type UpstreamConfig struct {
	Tag                string `yaml:"tag"`
	Addr               string `yaml:"addr"` // Required.
	DialAddr           string `yaml:"dial_addr"`
	IdleTimeout        int    `yaml:"idle_timeout"`
	MaxConns           int    `yaml:"max_conns"`
	EnablePipeline     bool   `yaml:"enable_pipeline"`
	EnableHTTP3        bool   `yaml:"enable_http3"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`

	Socks5       string `yaml:"socks5"`
	SoMark       int    `yaml:"so_mark"`
	BindToDevice string `yaml:"bind_to_device"`
	Bootstrap    string `yaml:"bootstrap"`
}

func Init(bp *coremain.BP, args any) (any, error) {
	f, err := NewForward(args.(*Args), Opts{Logger: bp.L(), MetricsTag: bp.Tag()})
	if err != nil {
		return nil, err
	}
	if err := f.RegisterMetricsTo(prometheus.WrapRegistererWithPrefix(PluginType+"_", bp.M().GetMetricsReg())); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

var _ sequence.Executable = (*Forward)(nil)
var _ sequence.QuickConfigurableExec = (*Forward)(nil)

type Forward struct {
	args *Args

	logger       *zap.Logger
	us           map[*upstreamWrapper]struct{}
	tag2Upstream map[string]*upstreamWrapper // for fast tag lookup only.
}

type Opts struct {
	Logger     *zap.Logger
	MetricsTag string
}

// NewForward inits a Forward from given args.
// args must contain at least one upstream.
func NewForward(args *Args, opt Opts) (*Forward, error) {
	if len(args.Upstreams) == 0 {
		return nil, errors.New("no upstream is configured")
	}
	if opt.Logger == nil {
		opt.Logger = zap.NewNop()
	}

	f := &Forward{
		args:         args,
		logger:       opt.Logger,
		us:           make(map[*upstreamWrapper]struct{}),
		tag2Upstream: make(map[string]*upstreamWrapper),
	}

	applyGlobal := func(c *UpstreamConfig) {
		utils.SetDefaultString(&c.Socks5, args.Socks5)
		utils.SetDefaultUnsignNum(&c.SoMark, args.SoMark)
		utils.SetDefaultString(&c.BindToDevice, args.BindToDevice)
		utils.SetDefaultString(&c.Bootstrap, args.Bootstrap)
	}

	for i, c := range args.Upstreams {
		if len(c.Addr) == 0 {
			return nil, fmt.Errorf("#%d upstream invalid args, addr is required", i)
		}
		applyGlobal(&c)
		uOpt := upstream.Opt{
			DialAddr:       c.DialAddr,
			Socks5:         c.Socks5,
			SoMark:         c.SoMark,
			BindToDevice:   c.BindToDevice,
			IdleTimeout:    time.Duration(c.IdleTimeout) * time.Second,
			MaxConns:       c.MaxConns,
			EnablePipeline: c.EnablePipeline,
			EnableHTTP3:    c.EnableHTTP3,
			Bootstrap:      c.Bootstrap,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: c.InsecureSkipVerify,
				ClientSessionCache: tls.NewLRUClientSessionCache(4),
			},
			Logger: opt.Logger,
		}

		u, err := upstream.NewUpstream(c.Addr, uOpt)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("failed to init upstream #%d: %w", i, err)
		}
		uw := wrapUpstream(u, c, opt.MetricsTag)
		f.us[uw] = struct{}{}

		if len(c.Tag) > 0 {
			if _, dup := f.tag2Upstream[c.Tag]; dup {
				_ = f.Close()
				return nil, fmt.Errorf("duplicated upstream tag %s", c.Tag)
			}
			f.tag2Upstream[c.Tag] = uw
		}
	}

	return f, nil
}

func (f *Forward) RegisterMetricsTo(r prometheus.Registerer) error {
	for wu := range f.us {
		// Only register metrics for upstream that has a tag.
		if len(wu.cfg.Tag) == 0 {
			continue
		}
		if err := wu.registerMetricsTo(r); err != nil {
			return err
		}
	}
	return nil
}

func (f *Forward) Exec(ctx context.Context, qCtx *query_context.Context) (err error) {
	r, err := f.exchange(ctx, qCtx, f.us)
	if err != nil {
		return err
	}
	qCtx.SetResponse(r)
	return nil
}

// QuickConfigureExec format: [upstream_tag]...
func (f *Forward) QuickConfigureExec(args string) (any, error) {
	var us map[*upstreamWrapper]struct{}
	if len(args) == 0 { // No args, use all upstreams.
		us = f.us
	} else { // Pick up upstreams by tags.
		us = make(map[*upstreamWrapper]struct{})
		for _, tag := range strings.Fields(args) {
			u := f.tag2Upstream[tag]
			if u == nil {
				return nil, fmt.Errorf("cannot find upstream by tag %s", tag)
			}
			us[u] = struct{}{}
		}
	}
	execFunc := func(ctx context.Context, qCtx *query_context.Context) error {
		r, err := f.exchange(ctx, qCtx, us)
		if err != nil {
			return err
		}
		qCtx.SetResponse(r)
		return nil
	}
	return sequence.ExecutableFunc(execFunc), nil
}

func (f *Forward) Close() error {
	for u := range f.us {
		_ = (*u).Close()
	}
	return nil
}

var ErrAllFailed = errors.New("all upstreams failed")

func (f *Forward) exchange(ctx context.Context, qCtx *query_context.Context, us map[*upstreamWrapper]struct{}) (*dns.Msg, error) {
	tn := f.args.Concurrent
	if tn <= 0 {
		tn = 1
	}
	if tn > len(us) {
		tn = len(us)
	}
	if tn > maxConcurrentQueries {
		tn = maxConcurrentQueries
	}

	if tn == 1 { // No concurrent queries.
		var u *upstreamWrapper
		for u = range us {
			break
		}
		return u.ExchangeContext(ctx, qCtx.Q())
	}

	c := make(chan *dns.Msg, 1) // Channel for responses.
	idleDo := safe_close.IdleDo{Do: func() {
		close(c)
	}}
	idleDo.Add(tn)

	qc := qCtx.Copy() // qCtx is not safe for concurrent use.
	i := 0
	for u := range us {
		i++
		if i > tn {
			break
		}

		u := u
		go func() {
			defer idleDo.Done()
			qCtx := qc

			// Give each upstream a fixed timeout to finsh the query.
			ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
			defer cancel()
			r, err := u.ExchangeContext(ctx, qCtx.Q())
			if err != nil {
				f.logger.Warn("upstream error", qCtx.InfoField(), zap.String("upstream", u.name()), zap.Error(err))
			}
			if r != nil {
				select {
				case c <- r:
				default:
				}
			}
		}()
	}

readLoop:
	for {
		select {
		case r, ok := <-c:
			if !ok {
				break readLoop
			}
			return r, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, ErrAllFailed
}

func quickSetup(bq sequence.BQ, s string) (any, error) {
	args := new(Args)
	args.Concurrent = maxConcurrentQueries
	for _, u := range strings.Fields(s) {
		args.Upstreams = append(args.Upstreams, UpstreamConfig{Addr: u})
	}
	return NewForward(args, Opts{Logger: bq.L()})
}
