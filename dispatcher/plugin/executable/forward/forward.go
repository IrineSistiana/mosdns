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
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
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
	upstream    []upstream.Upstream
	deduplicate bool

	sfGroup utils.ExchangeSingleFlightGroup
}

type Args struct {
	// options for dnsproxy upstream
	Upstream           []Upstream `yaml:"upstream"`
	Timeout            int        `yaml:"timeout"`
	InsecureSkipVerify bool       `yaml:"insecure_skip_verify"`
	Bootstrap          []string   `yaml:"bootstrap"`

	// options for mosdns
	Deduplicate bool `yaml:"deduplicate"`
}

type Upstream struct {
	Addr   string   `yaml:"addr"`
	IPAddr []string `yaml:"ip_addr"`
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newForwarder(bp, args.(*Args))
}

func newForwarder(bp *handler.BP, args *Args) (*forwarder, error) {
	if len(args.Upstream) == 0 {
		return nil, errors.New("no upstream is configured")
	}

	f := new(forwarder)
	f.BP = bp
	f.deduplicate = args.Deduplicate

	for _, u := range args.Upstream {
		if len(u.Addr) == 0 {
			return nil, errors.New("missing upstream address")
		}

		serverIPAddrs := make([]net.IP, 0, len(u.IPAddr))
		for _, s := range u.IPAddr {
			ip := net.ParseIP(s)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip addr %s", s)
			}
			serverIPAddrs = append(serverIPAddrs, ip)
		}

		opt := upstream.Options{}
		opt.Bootstrap = args.Bootstrap
		opt.ServerIPAddrs = serverIPAddrs

		opt.Timeout = time.Second * 10
		if args.Timeout > 0 {
			opt.Timeout = time.Second * time.Duration(args.Timeout)
		}

		opt.InsecureSkipVerify = args.InsecureSkipVerify

		u, err := upstream.AddressToUpstream(u.Addr, opt)
		if err != nil {
			return nil, fmt.Errorf("failed to init upsteam: %w", err)
		}

		f.upstream = append(f.upstream, u)
	}

	return f, nil
}

// Exec forwards qCtx.Q()() to upstreams, and sets qCtx.R().
// qCtx.Status() will be set as
// - handler.ContextStatusResponded: if it received a response.
// - handler.ContextStatusServerFailed: if all upstreams failed.
func (f *forwarder) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	return f.exec(ctx, qCtx)
}

func (f *forwarder) exec(ctx context.Context, qCtx *handler.Context) error {
	var r *dns.Msg
	var err error
	if f.deduplicate {
		r, err = f.sfGroup.Exchange(ctx, qCtx, f.exchange)
	} else {
		r, err = f.exchange(ctx, qCtx)
	}

	if err != nil {
		qCtx.SetResponse(nil, handler.ContextStatusServerFailed)
		return err
	}

	qCtx.SetResponse(r, handler.ContextStatusResponded)
	return nil
}

func (f *forwarder) exchange(_ context.Context, qCtx *handler.Context) (r *dns.Msg, err error) {
	r, u, err := upstream.ExchangeParallel(f.upstream, qCtx.Q())
	if err == nil {
		f.L().Debug("received response", qCtx.InfoField(), zap.String("from", u.Address()))
	}
	return r, err
}
