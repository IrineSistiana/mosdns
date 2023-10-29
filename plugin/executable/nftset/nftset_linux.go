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
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/nftset_utils"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/google/nftables"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"net/netip"
)

type nftsetPlugin struct {
	*coremain.BP
	args  *Args
	v4set *nftset_utils.NftSetHandler
	v6set *nftset_utils.NftSetHandler
	nc    *nftables.Conn
}

func newNftsetPlugin(bp *coremain.BP, args *Args) (*nftsetPlugin, error) {
	if m := args.Mask4; m <= 0 || m > 32 {
		args.Mask4 = 24
	}
	if m := args.Mask6; m <= 0 || m > 128 {
		args.Mask6 = 32
	}

	nc, err := nftables.New(nftables.AsLasting())
	if err != nil {
		return nil, fmt.Errorf("failed to connecet netlink, %w", err)
	}

	nftPlugin := &nftsetPlugin{
		BP:   bp,
		args: args,
		nc:   nc,
	}

	if len(args.TableFamily4) > 0 && len(args.TableName4) > 0 && len(args.SetName4) > 0 {
		f, ok := parseTableFamily(args.TableFamily4)
		if !ok {
			return nil, fmt.Errorf("unsupported nftables family for set4 [%s]", args.TableFamily4)
		}
		nftPlugin.v4set = nftset_utils.NewNtSetHandler(nftset_utils.HandlerOpts{
			Conn:        nc,
			TableFamily: f,
			TableName:   args.TableName4,
			SetName:     args.SetName4,
		})
	}

	if len(args.TableFamily6) > 0 && len(args.TableName6) > 0 && len(args.SetName6) > 0 {
		f, ok := parseTableFamily(args.TableFamily6)
		if !ok {
			return nil, fmt.Errorf("unsupported nftables family for set6 [%s]", args.TableFamily6)
		}
		nftPlugin.v6set = nftset_utils.NewNtSetHandler(nftset_utils.HandlerOpts{
			Conn:        nc,
			TableFamily: f,
			TableName:   args.TableName6,
			SetName:     args.SetName6,
		})
	}

	return nftPlugin, nil
}

// Exec tries to add all qCtx.R() IPs to system nftables.
// If an error occurred, Exec will just log it.
// Therefore, Exec will never raise its own error.
func (p *nftsetPlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	r := qCtx.R()
	if r != nil {
		er := p.addElems(r)
		if er != nil {
			p.L().Warn("failed to add elems to nftables", qCtx.InfoField(), zap.Error(er))
		}
	}

	return executable_seq.ExecChainNode(ctx, qCtx, next)
}

func (p *nftsetPlugin) addElems(r *dns.Msg) error {
	var v4Elems []netip.Prefix
	var v6Elems []netip.Prefix

	for i := range r.Answer {
		switch rr := r.Answer[i].(type) {
		case *dns.A:
			if p.v4set == nil {
				continue
			}
			addr, ok := netip.AddrFromSlice(rr.A)
			addr = addr.Unmap()
			if !ok || !addr.Is4() {
				return fmt.Errorf("internel: dns.A record [%s] is not a ipv4 address", rr.A)
			}
			v4Elems = append(v4Elems, netip.PrefixFrom(addr, p.args.Mask4))

		case *dns.AAAA:
			if p.v6set == nil {
				continue
			}
			addr, ok := netip.AddrFromSlice(rr.AAAA)
			if !ok {
				return fmt.Errorf("internel: dns.AAAA record [%s] is not a ipv6 address", rr.AAAA)
			}
			if addr.Is4() {
				addr = netip.AddrFrom16(addr.As16())
			}
			v6Elems = append(v6Elems, netip.PrefixFrom(addr, p.args.Mask6))
		default:
			continue
		}
	}

	if p.v4set != nil && len(v4Elems) > 0 {
		if err := p.v4set.AddElems(v4Elems...); err != nil {
			return fmt.Errorf("failed to add ipv4 elems %s: %w", v4Elems, err)
		}
	}

	if p.v6set != nil && len(v6Elems) > 0 {
		if err := p.v6set.AddElems(v6Elems...); err != nil {
			return fmt.Errorf("failed to add ipv6 elems %s: %w", v6Elems, err)
		}
	}
	return nil
}

func (p *nftsetPlugin) Close() error {
	return p.nc.CloseLasting()
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
