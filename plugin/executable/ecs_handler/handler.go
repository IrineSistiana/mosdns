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

package ecs_handler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const PluginType = "ecs_handler"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })

	// Compatible for old ecs plugin
	// TODO: Remove this in mosdns v6, probably.
	sequence.MustRegExecQuickSetup("ecs", QuickSetupOldECS)
}

var _ sequence.RecursiveExecutable = (*ECSHandler)(nil)

type Args struct {
	Forward bool   `yaml:"forward"`
	Send    bool   `yaml:"send"`
	Preset  string `yaml:"preset"`
	Mask4   int    `yaml:"mask4"`
	Mask6   int    `yaml:"mask6"`
}

type ECSHandler struct {
	args   Args
	preset netip.Addr // unmapped
}

func NewHandler(args Args) (*ECSHandler, error) {
	var preset netip.Addr
	if len(args.Preset) > 0 {
		addr, err := netip.ParseAddr(args.Preset)
		if err != nil {
			return nil, fmt.Errorf("invalid preset address, %w", err)
		}
		preset = addr.Unmap()
	}

	checkOrInitMask := func(p *int, min, max, defaultM int) bool {
		v := *p
		if v < min || v > max {
			return false
		}
		if v == 0 {
			*p = defaultM
		}
		return true
	}
	if !checkOrInitMask(&args.Mask4, 0, 32, 24) {
		return nil, errors.New("invalid mask4")
	}
	if !checkOrInitMask(&args.Mask6, 0, 128, 48) {
		return nil, errors.New("invalid mask6")
	}

	return &ECSHandler{args: args, preset: preset}, nil
}

func Init(_ *coremain.BP, args any) (any, error) {
	return NewHandler(*args.(*Args))
}

// Exec tries to append ECS to qCtx.Q().
func (e *ECSHandler) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	forwarded := e.addECS(qCtx)
	err := next.ExecNext(ctx, qCtx)
	if err != nil {
		return err
	}

	if forwarded {
		// forward upstream ecs back to client
		respOpt := qCtx.RespOpt()
		upstreamOpt := qCtx.UpstreamOpt()
		if respOpt != nil && upstreamOpt != nil {
			for _, o := range upstreamOpt.Option {
				if o.Option() == dns.EDNS0SUBNET {
					respOpt.Option = append(respOpt.Option, o)
					break
				}
			}
		}
	}
	return nil
}

// AddECS adds a *dns.EDNS0_SUBNET record to q.
func (e *ECSHandler) addECS(qCtx *query_context.Context) (forwarded bool) {
	queryOpt := qCtx.QOpt()
	// Check if query already has an ecs.
	for _, o := range queryOpt.Option {
		if o.Option() == dns.EDNS0SUBNET {
			return false // skip it
		}
	}
	if qCtx.QQuestion().Qclass != dns.ClassINET {
		// RFC 7871 5:
		// ECS is only defined for the Internet (IN) DNS class.
		return false
	}

	if e.args.Forward {
		clientOpt := qCtx.ClientOpt()
		if clientOpt != nil {
			for _, o := range clientOpt.Option {
				if o.Option() == dns.EDNS0SUBNET {
					queryOpt.Option = append(queryOpt.Option, o)
					return true
				}
			}
		}
	}

	if e.preset.IsValid() {
		clientAddr := e.preset
		var ecs *dns.EDNS0_SUBNET
		if clientAddr.Is4() {
			ecs = newSubnet(clientAddr.AsSlice(), uint8(e.args.Mask4), false)
		} else {
			ecs = newSubnet(clientAddr.AsSlice(), uint8(e.args.Mask6), true)
		}
		queryOpt.Option = append(queryOpt.Option, ecs)
		return false
	}

	if e.args.Send {
		clientAddr := qCtx.ServerMeta.ClientAddr
		if clientAddr.IsValid() {
			clientAddr = clientAddr.Unmap()
			var ecs *dns.EDNS0_SUBNET
			if clientAddr.Is4() {
				ecs = newSubnet(clientAddr.AsSlice(), uint8(e.args.Mask4), false)
			} else {
				ecs = newSubnet(clientAddr.AsSlice(), uint8(e.args.Mask6), true)
			}
			queryOpt.Option = append(queryOpt.Option, ecs)
			return false
		}
	}
	return false
}

func newSubnet(ip net.IP, mask uint8, v6 bool) *dns.EDNS0_SUBNET {
	edns0Subnet := new(dns.EDNS0_SUBNET)
	// edns family: https://www.iana.org/assignments/address-family-numbers/address-family-numbers.xhtml
	// ipv4 = 1
	// ipv6 = 2
	if !v6 { // ipv4
		edns0Subnet.Family = 1
	} else { // ipv6
		edns0Subnet.Family = 2
	}

	edns0Subnet.SourceNetmask = mask
	edns0Subnet.Code = dns.EDNS0SUBNET
	edns0Subnet.Address = ip

	// SCOPE PREFIX-LENGTH, an unsigned octet representing the leftmost
	// number of significant bits of ADDRESS that the response covers.
	// In queries, it MUST be set to 0.
	// https://tools.ietf.org/html/rfc7871
	edns0Subnet.SourceScope = 0
	return edns0Subnet
}

// QuickSetup format:
// old: [ip/mask] [ip/mask]
// new: [ip]
// Note: only the first ip will be used as preset address, the second one
// will be ignored. The mask value will be ignored.
func QuickSetupOldECS(bq sequence.BQ, s string) (any, error) {
	a := Args{}
	fs := strings.Fields(s)
	if len(fs) > 0 {
		var foundMask bool
		a.Preset, _, foundMask = strings.Cut(fs[0], "/")
		if foundMask {
			bq.L().Warn("ip mask value is deprecated and will be ignored. The default value (24/48) will be used")
		}
		if len(fs) > 1 {
			bq.L().Warn("Dual-stack ecs is deprecated. Only the first ip will be used as preset ecs address. Others will be simply ignored")
		}
	}
	return NewHandler(a)
}
