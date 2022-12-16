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

package ecs

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"net/netip"
	"strings"
)

const PluginType = "ecs"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.RecursiveExecutable = (*AddECS)(nil)

// QuickSetup format: [ip/mask] [ip/mask]
func QuickSetup(_ sequence.BQ, s string) (any, error) {
	var ipv4, ipv6 netip.Prefix
	for _, prefixStr := range strings.Fields(s) {
		prefix, err := netip.ParsePrefix(prefixStr)
		if err != nil {
			return nil, err
		}
		if prefix.Addr().Is4() {
			ipv4 = prefix
		} else {
			ipv6 = prefix
		}
	}
	return NewAddECS(ipv4, ipv6), nil
}

type AddECS struct {
	ipv4, ipv6 netip.Prefix
}

// NewAddECS returns a AddECS with given prefixes.
// If prefix is zero/invalid, it will be ignored.
func NewAddECS(ipv4, ipv6 netip.Prefix) *AddECS {
	return &AddECS{ipv4: ipv4, ipv6: ipv6}
}

// Exec tries to append ECS to qCtx.Q().
func (e *AddECS) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	upgraded, newECS := e.addECS(qCtx)
	err := next.ExecNext(ctx, qCtx)
	if err != nil {
		return err
	}

	if r := qCtx.R(); r != nil {
		if upgraded {
			dnsutils.RemoveEDNS0(r)
		} else {
			if newECS {
				dnsutils.RemoveMsgECS(r)
			}
		}
	}
	return nil
}

// AddECS adds a *dns.EDNS0_SUBNET record to q.
// upgraded: Whether the AddECS upgraded the q to a EDNS0 enabled query.
// newECS: Whether the AddECS added a *dns.EDNS0_SUBNET to q that didn't
// have a *dns.EDNS0_SUBNET before.
func (e *AddECS) addECS(qCtx *query_context.Context) (upgraded bool, newECS bool) {
	q := qCtx.Q()
	if len(q.Question) != 1 || q.Question[0].Qclass != dns.ClassINET {
		return false, false
	}
	qtype := q.Question[0].Qtype

	var ecs *dns.EDNS0_SUBNET
	switch qtype {
	case dns.TypeAAAA: // append ipv6 ecs first for AAAA type.
		if e.ipv6.IsValid() {
			ecs = dnsutils.NewEDNS0Subnet(e.ipv6.Addr().AsSlice(), uint8(e.ipv6.Bits()), true)
		} else if e.ipv4.IsValid() {
			ecs = dnsutils.NewEDNS0Subnet(e.ipv4.Addr().AsSlice(), uint8(e.ipv4.Bits()), false)
		}

	default: // append ipv4 first for other types, including A.
		if e.ipv4.IsValid() {
			ecs = dnsutils.NewEDNS0Subnet(e.ipv4.Addr().AsSlice(), uint8(e.ipv4.Bits()), false)
		} else if e.ipv6.IsValid() {
			ecs = dnsutils.NewEDNS0Subnet(e.ipv6.Addr().AsSlice(), uint8(e.ipv6.Bits()), true)
		}
	}

	opt := q.IsEdns0()
	if ecs != nil {
		if opt == nil {
			upgraded = true
			opt = dnsutils.UpgradeEDNS0(q)
		}
		newECS = dnsutils.AddECS(opt, ecs, true)
		return upgraded, newECS
	}
	return false, false
}
