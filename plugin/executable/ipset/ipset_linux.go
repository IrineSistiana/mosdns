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
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
	"github.com/nadoo/ipset"
	"net/netip"
)

type ipSetPlugin struct {
	args *Args
	nl   *ipset.NetLink
}

func newIpSetPlugin(args *Args) (*ipSetPlugin, error) {
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

	return &ipSetPlugin{
		args: args,
		nl:   nl,
	}, nil
}

func addIpSet(nl *ipset.NetLink, setName string, prefix netip.Prefix, timeout int) error {
	if timeout == -1 {
		return ipset.AddPrefix(nl, setName, prefix)
	}
	return ipset.AddPrefix(nl, setName, prefix, ipset.OptTimeout(uint32(timeout)))
}

func (p *ipSetPlugin) Exec(_ context.Context, qCtx *query_context.Context) error {
	r := qCtx.R()
	if r != nil {
		if err := p.addIPSet(r); err != nil {
			return fmt.Errorf("ipset: %w", err)
		}
	}
	return nil
}

func (p *ipSetPlugin) Close() error {
	return p.nl.Close()
}

func (p *ipSetPlugin) addIPSet(r *dns.Msg) error {
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
			if err := addIpSet(p.nl, p.args.SetName4, netip.PrefixFrom(addr, p.args.Mask4), p.args.Timeout4); err != nil {
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
			if err := addIpSet(p.nl, p.args.SetName6, netip.PrefixFrom(addr, p.args.Mask6), p.args.Timeout6); err != nil {
				return err
			}
		default:
			continue
		}
	}

	return nil
}
