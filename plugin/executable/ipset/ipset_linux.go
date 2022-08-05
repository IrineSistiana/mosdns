//go:build linux

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

package ipset

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/miekg/dns"
	"github.com/nadoo/ipset"
	"go.uber.org/zap"
	"net/netip"
)

var _ coremain.ExecutablePlugin = (*ipsetPlugin)(nil)

type ipsetPlugin struct {
	*coremain.BP
	args *Args
	nl   *ipset.NetLink
}

func newIpsetPlugin(bp *coremain.BP, args *Args) (*ipsetPlugin, error) {
	if args.Mask4 == 0 {
		args.Mask4 = 24
	}
	if args.Mask6 == 0 {
		args.Mask6 = 32
	}

	nl, err := ipset.Init()
	if err != nil {
		return nil, err
	}

	return &ipsetPlugin{
		BP:   bp,
		args: args,
		nl:   nl,
	}, nil
}

func (p *ipsetPlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	r := qCtx.R()
	if r != nil {
		er := p.addIPSet(r)
		if er != nil {
			p.L().Warn("failed to add response IP to ipset", qCtx.InfoField(), zap.Error(er))
		}
	}

	return executable_seq.ExecChainNode(ctx, qCtx, next)
}

func (p *ipsetPlugin) Close() error {
	return p.nl.Close()
}

func (p *ipsetPlugin) addIPSet(r *dns.Msg) error {
	for i := range r.Answer {
		switch rr := r.Answer[i].(type) {
		case *dns.A:
			if len(p.args.SetName4) == 0 {
				continue
			}
			addr, ok := netip.AddrFromSlice(rr.A.To4())
			if !ok {
				return fmt.Errorf("invalid A record with ip: %s", rr.A)
			}
			if err := ipset.AddPrefix(p.nl, p.args.SetName4, netip.PrefixFrom(addr, p.args.Mask4)); err != nil {
				return err
			}

		case *dns.AAAA:
			if len(p.args.SetName6) == 0 {
				continue
			}
			addr, ok := netip.AddrFromSlice(rr.AAAA.To16())
			if !ok {
				return fmt.Errorf("invalid AAAA record with ip: %s", rr.AAAA)
			}
			if err := ipset.AddPrefix(p.nl, p.args.SetName6, netip.PrefixFrom(addr, p.args.Mask6)); err != nil {
				return err
			}
		default:
			continue
		}
	}

	return nil
}
