//     Copyright (C) 2020, IrineSistiana
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

package ecs

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
)

const PluginType = "ecs"

func init() {
	handler.RegInitFunc(PluginType, Init)

	handler.MustRegPlugin(handler.WrapFunctionalPlugin("_auto_ecs", PluginType, &ecsPlugin{args: &Args{Auto: true}}))
}

var _ handler.Functional = (*ecsPlugin)(nil)

type Args struct {
	// Automatically append client address as ecs.
	// If this is true, pre-set addresses will not be used.
	Auto bool `yaml:"auto"`

	// force over write existing ecs
	ForceOverwrite bool `yaml:"force_overwrite"`

	// mask for ecs
	Mask4 uint8 `yaml:"mask4"`
	Mask6 uint8 `yaml:"mask6"`

	// pre-set address
	IPv4 string `yaml:"ipv4"`
	IPv6 string `yaml:"ipv6"`
}

type ecsPlugin struct {
	args       *Args
	ipv4, ipv6 *dns.EDNS0_SUBNET
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	ep := new(ecsPlugin)
	ep.args = args

	if len(args.IPv4) != 0 {
		ip := net.ParseIP(args.IPv4)
		if ip == nil {
			return nil, fmt.Errorf("invaild ipv4 address %s", args.IPv4)
		}
		if ip4 := ip.To4(); ip4 == nil {
			return nil, fmt.Errorf("%s is not a ipv4 address", args.IPv4)
		} else {
			ep.ipv4 = newEDNS0Subnet(ip4, args.Mask4, false)
		}
	}

	if len(args.IPv6) != 0 {
		ip := net.ParseIP(args.IPv6)
		if ip == nil {
			return nil, fmt.Errorf("invaild ipv6 address %s", args.IPv6)
		}
		if ip6 := ip.To16(); ip6 == nil {
			return nil, fmt.Errorf("%s is not a ipv6 address", args.IPv6)
		} else {
			ep.ipv6 = newEDNS0Subnet(ip6, args.Mask6, true)
		}
	}

	return handler.WrapFunctionalPlugin(tag, PluginType, ep), nil
}

// Do tries to append ECS to qCtx.Q.
// If an error occurred, Do will just log it.
// Therefore, Do will never return a err.
func (e ecsPlugin) Do(_ context.Context, qCtx *handler.Context) (err error) {
	if qCtx == nil || qCtx.Q == nil {
		return nil
	}

	if checkMsgHasECS(qCtx.Q) == false || e.args.ForceOverwrite {
		if e.args.Auto && qCtx.From != nil {
			ip, err := utils.GetIPFromAddr(qCtx.From)
			if err != nil {
				qCtx.Logf(logrus.WarnLevel, "internal err: can not get ip address from qCtx.From [%s]", qCtx.From)
				return nil
			}
			var ecs *dns.EDNS0_SUBNET
			if ip4 := ip.To4(); ip4 != nil { // is ipv4
				ecs = newEDNS0Subnet(ip, e.args.Mask4, false)
			} else {
				if ip6 := ip.To16(); ip6 != nil { // is ipv6
					ecs = newEDNS0Subnet(ip, e.args.Mask6, true)
				} else { // non
					qCtx.Logf(logrus.WarnLevel, "internal err: client address [%s] is not a valid ip address", qCtx.From)
					return nil
				}
			}
			setECS(qCtx.Q, ecs)
		} else {
			switch {
			case e.ipv4 != nil && checkQueryType(qCtx.Q, dns.TypeA):
				setECS(qCtx.Q, e.ipv4)
			case e.ipv6 != nil && checkQueryType(qCtx.Q, dns.TypeAAAA):
				setECS(qCtx.Q, e.ipv6)
			}
		}
	}
	return nil
}

func checkQueryType(m *dns.Msg, typ uint16) bool {
	if len(m.Question) > 0 && m.Question[0].Qtype == typ {
		return true
	}
	return false
}
