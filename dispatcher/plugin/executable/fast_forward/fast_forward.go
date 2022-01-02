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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/bundled_upstream"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/pool"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/upstream"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"time"
)

const PluginType = "fast_forward"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*fastForward)(nil)

type fastForward struct {
	*handler.BP
	args *Args

	upstreamBundle  *bundled_upstream.BundledUpstream
	trackedUpstream []upstream.Upstream
}

type Args struct {
	Upstream []*UpstreamConfig `yaml:"upstream"`
	Timeout  int               `yaml:"timeout"`
	CA       []string          `yaml:"ca"`
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
		BP:   bp,
		args: args,
	}

	us := make([]bundled_upstream.Upstream, 0)

	// rootCAs
	var rootCAs *x509.CertPool
	if len(args.CA) != 0 {
		var err error
		rootCAs, err = utils.LoadCertPool(args.CA)
		if err != nil {
			return nil, fmt.Errorf("failed to load ca: %w", err)
		}
	}

	for i, c := range args.Upstream {
		if len(c.Addr) == 0 {
			return nil, errors.New("missing server addr")
		}

		opt := &upstream.Opt{
			DialAddr:    c.DialAddr,
			Socks5:      c.Socks5,
			IdleTimeout: time.Duration(c.IdleTimeout) * time.Second,
			MaxConns:    c.MaxConns,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: c.InsecureSkipVerify,
				RootCAs:            rootCAs,
				ClientSessionCache: tls.NewLRUClientSessionCache(64),
			},
			Logger: bp.L(),
		}

		u, err := upstream.NewUpstream(c.Addr, opt)

		if err != nil {
			return nil, fmt.Errorf("failed to init upstream: %w", err)
		}

		wu := &upstreamWrapper{
			address: c.Addr,
			trusted: c.Trusted,
			u:       u,
		}

		if i == 0 { // Set first upstream as trusted upstream.
			wu.trusted = true
		}

		us = append(us, wu)
		f.trackedUpstream = append(f.trackedUpstream, u)
	}

	f.upstreamBundle = bundled_upstream.NewBundledUpstream(us, bp.L())
	return f, nil
}

type upstreamWrapper struct {
	address string
	trusted bool
	u       upstream.Upstream
}

func (u *upstreamWrapper) Exchange(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	qRaw, buf, err := pool.PackBuffer(q)
	if err != nil {
		return nil, err
	}
	defer pool.ReleaseBuf(buf)
	rRaw, err := u.u.ExchangeContext(ctx, qRaw)
	if err != nil {
		return nil, err
	}
	r := new(dns.Msg)
	if err := r.Unpack(rRaw); err != nil {
		return nil, err
	}
	return r, nil
}

func (u *upstreamWrapper) Address() string {
	return u.address
}

func (u *upstreamWrapper) Trusted() bool {
	return u.trusted
}

// Exec forwards qCtx.Q() to upstreams, and sets qCtx.R().
// qCtx.Status() will be set as
// - handler.ContextStatusResponded: if it received a response.
// - handler.ContextStatusServerFailed: if all upstreams failed.
func (f *fastForward) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	err := f.exec(ctx, qCtx)
	if err != nil {
		return err
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}

func (f *fastForward) exec(ctx context.Context, qCtx *handler.Context) (err error) {
	r, err := f.upstreamBundle.ExchangeParallel(ctx, qCtx)
	if err != nil {
		qCtx.SetResponse(nil, handler.ContextStatusServerFailed)
		return err
	}

	qCtx.SetResponse(r, handler.ContextStatusResponded)
	return nil
}

func (f *fastForward) Shutdown() error {
	for _, u := range f.trackedUpstream {
		u.CloseIdleConnections()
	}
	return nil
}
