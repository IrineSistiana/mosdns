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

package nftset

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/IrineSistiana/mosdns/v5/pkg/nftset_utils"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/google/nftables"
	"github.com/miekg/dns"
)

type nftSetPlugin struct {
	args      *Args
	v4Handler *nftset_utils.NftSetHandler
	v6Handler *nftset_utils.NftSetHandler
}

func newNftSetPlugin(args *Args) (*nftSetPlugin, error) {
	utils.SetDefaultUnsignNum(&args.IPv4.Mask, 24)
	utils.SetDefaultUnsignNum(&args.IPv6.Mask, 48)
	if m := args.IPv4.Mask; m > 32 {
		return nil, fmt.Errorf("invalid ipv4 mask %d", m)
	}
	if m := args.IPv6.Mask; m > 128 {
		return nil, fmt.Errorf("invalid ipv6 mask %d", m)
	}

	p := &nftSetPlugin{
		args: args,
	}

	newHandler := func(sa SetArgs) (*nftset_utils.NftSetHandler, error) {
		if !(len(sa.Table) > 0 && len(sa.TableFamily) > 0 && len(sa.Set) > 0) {
			return nil, nil
		}
		f, ok := parseTableFamily(sa.TableFamily)
		if !ok {
			return nil, fmt.Errorf("unsupported nftables family [%s]", sa.TableFamily)
		}
		return nftset_utils.NewNtSetHandler(nftset_utils.HandlerOpts{
			TableFamily: f,
			TableName:   sa.Table,
			SetName:     sa.Set,
		}), nil
	}
	var err error
	p.v4Handler, err = newHandler(args.IPv4)
	if err != nil {
		return nil, err
	}
	p.v6Handler, err = newHandler(args.IPv6)
	if err != nil {
		_ = p.v4Handler.Close()
		return nil, err
	}
	return p, nil
}

func (p *nftSetPlugin) Exec(_ context.Context, qCtx *query_context.Context) error {
	r := qCtx.R()
	if r != nil {
		if err := p.addElems(r); err != nil {
			return fmt.Errorf("nftable: %w", err)
		}
	}
	return nil
}

func (p *nftSetPlugin) addElems(r *dns.Msg) error {
	var v4Elems []netip.Prefix
	var v6Elems []netip.Prefix

	for i := range r.Answer {
		switch rr := r.Answer[i].(type) {
		case *dns.A:
			if p.v4Handler == nil {
				continue
			}
			addr, ok := netip.AddrFromSlice(rr.A)
			addr = addr.Unmap()
			if !ok || !addr.Is4() {
				return fmt.Errorf("internel: dns.A record [%s] is not a ipv4 address", rr.A)
			}
			v4Elems = append(v4Elems, netip.PrefixFrom(addr, p.args.IPv4.Mask))

		case *dns.AAAA:
			if p.v6Handler == nil {
				continue
			}
			addr, ok := netip.AddrFromSlice(rr.AAAA)
			if !ok {
				return fmt.Errorf("internel: dns.AAAA record [%s] is not a ipv6 address", rr.AAAA)
			}
			if addr.Is4() {
				addr = netip.AddrFrom16(addr.As16())
			}
			v6Elems = append(v6Elems, netip.PrefixFrom(addr, p.args.IPv6.Mask))
		default:
			continue
		}
	}

	if p.v4Handler != nil && len(v4Elems) > 0 {
		if err := p.v4Handler.AddElems(v4Elems...); err != nil {
			return fmt.Errorf("failed to add ipv4 elems %s: %w", v4Elems, err)
		}
	}

	if p.v6Handler != nil && len(v6Elems) > 0 {
		if err := p.v6Handler.AddElems(v6Elems...); err != nil {
			return fmt.Errorf("failed to add ipv6 elems %s: %w", v6Elems, err)
		}
	}
	return nil
}

func (p *nftSetPlugin) Close() error {
	if p.v4Handler != nil {
		_ = p.v4Handler.Close()
	}
	if p.v6Handler != nil {
		_ = p.v6Handler.Close()
	}
	return nil
}

func parseTableFamily(s string) (nftables.TableFamily, bool) {
	switch s {
	case "ip":
		return nftables.TableFamilyIPv4, true
	case "ip6":
		return nftables.TableFamilyIPv6, true
	case "inet":
		return nftables.TableFamilyINet, true
	default:
		return 0, false
	}
}
