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

package forward_dnsproxy

import (
	"context"
	"errors"
	"fmt"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"net"
	"time"
)

const PluginType = "forward_dnsproxy"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ sequence.Executable = (*forwardPlugin)(nil)

type forwardPlugin struct {
	*coremain.BP

	upstreams []upstream.Upstream
}

type Args struct {
	// options for dnsproxy upstream
	Upstreams          []UpstreamConfig `yaml:"upstreams"`
	Timeout            int              `yaml:"timeout"`
	InsecureSkipVerify bool             `yaml:"insecure_skip_verify"`
	Bootstrap          []string         `yaml:"bootstrap"`
}

type UpstreamConfig struct {
	Addr    string   `yaml:"addr"`
	IPAddrs []string `yaml:"ip_addrs"`
}

func Init(bp *coremain.BP, args interface{}) (coremain.Plugin, error) {
	return newForwarder(bp, args.(*Args))
}

func newForwarder(bp *coremain.BP, args *Args) (*forwardPlugin, error) {
	if len(args.Upstreams) == 0 {
		return nil, errors.New("no upstream is configured")
	}

	f := new(forwardPlugin)
	f.BP = bp

	for i, conf := range args.Upstreams {
		serverIPAddrs := make([]net.IP, 0, len(conf.IPAddrs))
		for _, s := range conf.IPAddrs {
			ip := net.ParseIP(s)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip addr %s", s)
			}
			serverIPAddrs = append(serverIPAddrs, ip)
		}

		opt := &upstream.Options{}
		opt.Bootstrap = args.Bootstrap
		opt.ServerIPAddrs = serverIPAddrs

		opt.Timeout = time.Second * 10
		if args.Timeout > 0 {
			opt.Timeout = time.Second * time.Duration(args.Timeout)
		}

		opt.InsecureSkipVerify = args.InsecureSkipVerify

		u, err := upstream.AddressToUpstream(conf.Addr, opt)
		if err != nil {
			return nil, fmt.Errorf("failed to init upsteam #%d: %w", i, err)
		}
		f.upstreams = append(f.upstreams, u)
	}
	return f, nil
}

func (f *forwardPlugin) Exec(ctx context.Context, qCtx *query_context.Context) error {
	return f.exec(ctx, qCtx)
}

func (f *forwardPlugin) exec(ctx context.Context, qCtx *query_context.Context) error {
	type res struct {
		r   *dns.Msg
		err error
	}
	// Remainder: Always makes a copy of q. dnsproxy/upstream may keep or even modify the q in their
	// Exchange() calls.
	q := qCtx.Q().Copy()
	c := make(chan res, 1)
	go func() {
		r, _, err := upstream.ExchangeParallel(f.upstreams, q)
		c <- res{
			r:   r,
			err: err,
		}
	}()

	select {
	case res := <-c:
		if res.err != nil {
			return res.err
		}
		qCtx.SetResponse(res.r)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *forwardPlugin) Close() error {
	for _, u := range f.upstreams {
		u.Close()
	}
	return nil
}
