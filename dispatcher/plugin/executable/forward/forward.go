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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"net"
	"time"
)

const PluginType = "forward"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*forwarder)(nil)

type forwarder struct {
	*handler.BP
	upstream    []utils.Upstream
	deduplicate bool

	fastIPHandler  *fastip.FastestAddr // nil if fast ip is disabled
	upstreamFastIP []upstream.Upstream // same as upstream, just used by fastIPHandler

	sfGroup utils.ExchangeSingleFlightGroup
}

type Args struct {
	// options for dnsproxy upstream
	UpstreamConfig     []UpstreamConfig `yaml:"upstream"`
	Timeout            int              `yaml:"timeout"`
	InsecureSkipVerify bool             `yaml:"insecure_skip_verify"`
	Bootstrap          []string         `yaml:"bootstrap"`
	FastestIP          bool             `yaml:"fastest_ip"`

	// options for mosdns
	Deduplicate bool `yaml:"deduplicate"`
}

type UpstreamConfig struct {
	Addr    string   `yaml:"addr"`
	IPAddr  []string `yaml:"ip_addr"`
	Trusted bool     `yaml:"trusted"`
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newForwarder(bp, args.(*Args))
}

func newForwarder(bp *handler.BP, args *Args) (*forwarder, error) {
	if len(args.UpstreamConfig) == 0 {
		return nil, errors.New("no upstream is configured")
	}

	f := new(forwarder)
	f.BP = bp
	f.deduplicate = args.Deduplicate

	if args.FastestIP {
		f.fastIPHandler = fastip.NewFastestAddr()
	}

	for _, conf := range args.UpstreamConfig {
		if len(conf.Addr) == 0 {
			return nil, errors.New("missing upstream address")
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

		if args.FastestIP {
			f.upstreamFastIP = append(f.upstreamFastIP, u)
		} else {
			f.upstream = append(f.upstream, &trustedUpstream{
				dnsproxyUpstream: u,
				trusted:          conf.Trusted,
			})
		}
	}

	return f, nil
}

type trustedUpstream struct {
	dnsproxyUpstream upstream.Upstream
	trusted          bool
}

func (u *trustedUpstream) Address() string {
	return u.dnsproxyUpstream.Address()
}

func (u *trustedUpstream) Exchange(qCtx *handler.Context) (*dns.Msg, error) {
	return u.dnsproxyUpstream.Exchange(qCtx.Q())
}

func (u *trustedUpstream) Trusted() bool {
	return u.trusted
}

// Exec forwards qCtx.Q() to upstreams, and sets qCtx.R().
// qCtx.Status() will be set as
// - handler.ContextStatusResponded: if it received a response.
// - handler.ContextStatusServerFailed: if all upstreams failed.
func (f *forwarder) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	err := f.exec(ctx, qCtx)
	if err != nil {
		return err
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}

func (f *forwarder) exec(ctx context.Context, qCtx *handler.Context) error {
	var r *dns.Msg
	var err error
	if f.fastIPHandler != nil {
		r, _, err = f.fastIPHandler.ExchangeFastest(qCtx.Q().Copy(), f.upstreamFastIP)
	} else if f.deduplicate {
		r, err = f.sfGroup.Exchange(ctx, qCtx, f.upstream, f.L())
	} else {
		r, err = utils.ExchangeParallel(ctx, qCtx, f.upstream, f.L())
	}

	if err != nil {
		qCtx.SetResponse(nil, handler.ContextStatusServerFailed)
		return err
	}

	qCtx.SetResponse(r, handler.ContextStatusResponded)
	return nil
}
