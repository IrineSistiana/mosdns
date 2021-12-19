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

package forward

import (
	"context"
	"errors"
	"fmt"
	"github.com/AdguardTeam/dnsproxy/fastip"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/bundled_upstream"
	"github.com/miekg/dns"
	"net"
	"time"
)

const PluginType = "forward"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*forwardPlugin)(nil)

type forwardPlugin struct {
	*handler.BP

	upstreams []upstream.Upstream // same as upstream, just used by fastIPHandler
	bu        *bundled_upstream.BundledUpstream

	fastIPHandler *fastip.FastestAddr // nil if fast ip is disabled
}

type Args struct {
	// options for dnsproxy upstream
	UpstreamConfig     []UpstreamConfig `yaml:"upstream"`
	Timeout            int              `yaml:"timeout"`
	InsecureSkipVerify bool             `yaml:"insecure_skip_verify"`
	Bootstrap          []string         `yaml:"bootstrap"`
	FastestIP          bool             `yaml:"fastest_ip"`
}

type UpstreamConfig struct {
	Addr    string   `yaml:"addr"`
	IPAddr  []string `yaml:"ip_addr"`
	Trusted bool     `yaml:"trusted"`
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newForwarder(bp, args.(*Args))
}

func newForwarder(bp *handler.BP, args *Args) (*forwardPlugin, error) {
	if len(args.UpstreamConfig) == 0 {
		return nil, errors.New("no upstream is configured")
	}

	f := new(forwardPlugin)
	f.BP = bp

	bu := make([]bundled_upstream.Upstream, 0)
	for i, conf := range args.UpstreamConfig {
		if len(conf.Addr) == 0 {
			return nil, errors.New("missing upstream address")
		}

		if i == 0 { // Set first upstream as trusted upstream.
			conf.Trusted = true
		}

		serverIPAddrs := make([]net.IP, 0, len(conf.IPAddr))
		for _, s := range conf.IPAddr {
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
			return nil, fmt.Errorf("failed to init upsteam: %w", err)
		}

		f.upstreams = append(f.upstreams, u)
		bu = append(bu, &upstreamWrapper{
			dnsproxyUpstream: u,
			trusted:          conf.Trusted,
		})
	}

	if args.FastestIP {
		f.fastIPHandler = fastip.NewFastestAddr()
	} else {
		f.bu = bundled_upstream.NewBundledUpstream(bu, bp.L())
	}

	return f, nil
}

type upstreamWrapper struct {
	dnsproxyUpstream upstream.Upstream
	trusted          bool
}

func (u *upstreamWrapper) Address() string {
	return u.dnsproxyUpstream.Address()
}

func (u *upstreamWrapper) Exchange(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
	return u.dnsproxyUpstream.Exchange(q)
}

func (u *upstreamWrapper) Trusted() bool {
	return u.trusted
}

// Exec forwards qCtx.Q() to upstreams, and sets qCtx.R().
// qCtx.Status() will be set as
// - handler.ContextStatusResponded: if it received a response.
// - handler.ContextStatusServerFailed: if all upstreams failed.
func (f *forwardPlugin) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	err := f.exec(ctx, qCtx)
	if err != nil {
		return err
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}

func (f *forwardPlugin) exec(ctx context.Context, qCtx *handler.Context) error {
	var r *dns.Msg
	var err error
	if f.fastIPHandler != nil {
		r, _, err = f.fastIPHandler.ExchangeFastest(qCtx.Q().Copy(), f.upstreams)
	} else {
		r, err = f.bu.ExchangeParallel(ctx, qCtx)
	}

	if err != nil {
		qCtx.SetResponse(nil, handler.ContextStatusServerFailed)
		return err
	}

	qCtx.SetResponse(r, handler.ContextStatusResponded)
	return nil
}
