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

package fastforward

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"time"
)

const PluginType = "fast_forward"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ESExecutablePlugin = (*fastForward)(nil)

type fastForward struct {
	*handler.BP
	args *Args

	upstream []utils.Upstream

	sfGroup utils.ExchangeSingleFlightGroup
}

type Args struct {
	Deduplicate bool              `yaml:"deduplicate"`
	CA          []string          `yaml:"ca"`
	Timeout     int               `yaml:"timeout"`
	Upstream    []*UpstreamConfig `yaml:"upstream"`
}

type UpstreamConfig struct {
	Addr     string `yaml:"addr"` // required
	DialAddr string `yaml:"dial_addr"`
	Trusted  bool   `yaml:"trusted"`
	Socks5   string `yaml:"socks5"`

	IdleTimeout        int  `yaml:"idle_timeout"`
	MaxConns           int  `yaml:"max_conns"`
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newFastForward(bp, args.(*Args))
}

func newFastForward(bp *handler.BP, args *Args) (*fastForward, error) {
	if len(args.Upstream) == 0 {
		return nil, errors.New("no upstream is configured")
	}

	f := &fastForward{
		BP:       bp,
		args:     args,
		upstream: make([]utils.Upstream, 0),
	}

	// rootCAs
	var rootCAs *x509.CertPool
	if len(args.CA) != 0 {
		var err error
		rootCAs, err = utils.LoadCertPool(args.CA)
		if err != nil {
			return nil, fmt.Errorf("failed to load ca: %w", err)
		}
	}

	for _, config := range args.Upstream {
		if len(config.Addr) == 0 {
			return nil, errors.New("missing server addr")
		}

		u, err := upstream.NewFastUpstream(
			config.Addr,
			upstream.WithDialAddr(config.DialAddr),
			upstream.WithSocks5(config.Socks5),
			upstream.WithReadTimeout(time.Duration(args.Timeout)*time.Second),
			upstream.WithIdleTimeout(time.Duration(config.IdleTimeout)*time.Second),
			upstream.WithMaxConns(config.MaxConns),
			upstream.WithInsecureSkipVerify(config.InsecureSkipVerify),
			upstream.WithRootCAs(rootCAs),
			upstream.WithLogger(bp.L()),
		)

		if err != nil {
			return nil, fmt.Errorf("failed to init upstream: %w", err)
		}

		wu := &upstreamWrapper{
			trusted: config.Trusted,
			u:       u,
		}

		f.upstream = append(f.upstream, wu)
	}

	return f, nil
}

type upstreamWrapper struct {
	trusted bool
	u       *upstream.FastUpstream
}

func (u *upstreamWrapper) Exchange(qCtx *handler.Context) (*dns.Msg, error) {
	return u.u.Exchange(qCtx.Q())
}

func (u *upstreamWrapper) Address() string {
	return u.u.Address()
}

func (u *upstreamWrapper) Trusted() bool {
	return u.trusted
}

// ExecES forwards qCtx.Q() to upstreams, and sets qCtx.R().
// qCtx.Status() will be set as
// - handler.ContextStatusResponded: if it received a response.
// - handler.ContextStatusServerFailed: if all upstreams failed.
func (f *fastForward) ExecES(ctx context.Context, qCtx *handler.Context) (earlyStop bool, err error) {
	return false, f.exec(ctx, qCtx)
}

func (f *fastForward) exec(ctx context.Context, qCtx *handler.Context) (err error) {
	r, err := f.exchange(ctx, qCtx)
	if err != nil {
		qCtx.SetResponse(nil, handler.ContextStatusServerFailed)
		return err
	}

	qCtx.SetResponse(r, handler.ContextStatusResponded)
	return nil
}

func (f *fastForward) exchange(ctx context.Context, qCtx *handler.Context) (r *dns.Msg, err error) {
	if f.args.Deduplicate {
		return f.sfGroup.Exchange(ctx, qCtx, f.upstream, f.L())
	}
	return utils.ExchangeParallel(ctx, qCtx, f.upstream, f.L())
}
