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

const (
	queryTimeout = time.Second * 5
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.Executable = (*Forward)(nil)

type Forward struct {
	upstreams []upstream.Upstream
}

type Args struct {
	// options for dnsproxy upstream
	Upstreams          []UpstreamConfig `yaml:"upstreams"`
	InsecureSkipVerify bool             `yaml:"insecure_skip_verify"`
	Bootstrap          []string         `yaml:"bootstrap"`
}

type UpstreamConfig struct {
	Addr    string   `yaml:"addr"`
	IPAddrs []string `yaml:"ip_addrs"`
}

func Init(_ *coremain.BP, args any) (any, error) {
	return NewForward(args.(*Args))
}

// NewForward returns a Forward with given args.
// args must contain at least one upstream.
func NewForward(args *Args) (*Forward, error) {
	if len(args.Upstreams) == 0 {
		return nil, errors.New("no upstream is configured")
	}

	f := new(Forward)
	for i, conf := range args.Upstreams {
		serverIPAddrs := make([]net.IP, 0, len(conf.IPAddrs))
		for _, s := range conf.IPAddrs {
			ip := net.ParseIP(s)
			if ip == nil {
				_ = f.Close()
				return nil, fmt.Errorf("invalid ip addr %s", s)
			}
			serverIPAddrs = append(serverIPAddrs, ip)
		}

		opt := &upstream.Options{
			Bootstrap:          args.Bootstrap,
			Timeout:            queryTimeout,
			ServerIPAddrs:      serverIPAddrs,
			InsecureSkipVerify: args.InsecureSkipVerify,
		}
		u, err := upstream.AddressToUpstream(conf.Addr, opt)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("failed to init upsteam #%d: %w", i, err)
		}
		f.upstreams = append(f.upstreams, u)
	}
	return f, nil
}

func (f *Forward) Exec(ctx context.Context, qCtx *query_context.Context) error {
	r, _, err := f.Exchange(ctx, qCtx.Q())
	if err != nil {
		return err
	}
	if r != nil {
		qCtx.SetResponse(r)
	}
	return nil
}

func (f *Forward) Exchange(ctx context.Context, q *dns.Msg) (*dns.Msg, upstream.Upstream, error) {
	type res struct {
		r   *dns.Msg
		u   upstream.Upstream
		err error
	}
	// Remainder: Always makes a copy of q. dnsproxy/upstream may keep or even modify the q in their
	// Exchange() calls.
	qc := q.Copy()
	c := make(chan res, 1)
	go func() {
		r, u, err := upstream.ExchangeParallel(f.upstreams, qc)
		c <- res{
			r:   r,
			u:   u,
			err: err,
		}
	}()

	select {
	case res := <-c:
		return res.r, res.u, res.err
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

func (f *Forward) Close() error {
	for _, u := range f.upstreams {
		_ = u.Close()
	}
	return nil
}
