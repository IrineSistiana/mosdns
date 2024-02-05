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

package forward

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
)

const PluginType = "forward"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ coremain.ExecutablePlugin = (*forwardPlugin)(nil)

type forwardPlugin struct {
	*coremain.BP

	upstreams []upstream.Upstream
}

type Args struct {
	// options for dnsproxy upstream
	UpstreamConfig     []UpstreamConfig `yaml:"upstream"`
	Timeout            int              `yaml:"timeout"`
	InsecureSkipVerify bool             `yaml:"insecure_skip_verify"`
	Bootstrap          []string         `yaml:"bootstrap"`
	TrustCA            string           `yaml:"trust_ca"`
}

type UpstreamConfig struct {
	Addr   string   `yaml:"addr"`
	IPAddr []string `yaml:"ip_addr"`

	// Deprecated: This field is preserved. It has no effect.
	// TODO: Remove it in v5.
	Trusted bool `yaml:"trusted"`
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newForwarder(bp, args.(*Args))
}

func newForwarder(bp *coremain.BP, args *Args) (*forwardPlugin, error) {
	if len(args.UpstreamConfig) == 0 {
		return nil, errors.New("no upstream is configured")
	}

	f := new(forwardPlugin)
	f.BP = bp

	for i, conf := range args.UpstreamConfig {
		if len(conf.Addr) == 0 {
			return nil, fmt.Errorf("upstream #%d, missing upstream address", i)
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

		// Get the SystemCertPool, continue with an empty pool on error
		rootCAs, _ := x509.SystemCertPool()
		if rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}

		// Read in the cert file
		if args.TrustCA != "" {
			cert, err := os.ReadFile(args.TrustCA)
			if err != nil {
				return nil, fmt.Errorf("Failed to read custom CA file: %q", args.TrustCA)
			}

			// Append our cert to the system pool
			if ok := rootCAs.AppendCertsFromPEM(cert); !ok {
				return nil, fmt.Errorf("Failed to append %s to RootCAs", args.TrustCA)
			}
		}

		// set upstream's CA option
		opt.RootCAs = rootCAs

		u, err := upstream.AddressToUpstream(conf.Addr, opt)
		if err != nil {
			return nil, fmt.Errorf("failed to init upsteam #%d: %w", i, err)
		}
		f.upstreams = append(f.upstreams, u)
	}
	return f, nil
}

// Exec forwards qCtx.Q() to upstreams, and sets qCtx.R().
// qCtx.Status() will be set as
// - handler.ContextStatusResponded: if it received a response.
// - handler.ContextStatusServerFailed: if all upstreams failed.
func (f *forwardPlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	err := f.exec(ctx, qCtx)
	if err != nil {
		return err
	}

	return executable_seq.ExecChainNode(ctx, qCtx, next)
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
