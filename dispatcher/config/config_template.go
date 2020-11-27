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

package config

import (
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/blackhole"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/ecs"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/forward"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/ipset"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/redirect_domain"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/redirect_ip"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/redirect_query_type"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/sequence"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
)

func GetTemplateConfig() (*Config, error) {
	c := new(Config)
	c.Server.Bind = []string{"udp://0.0.0.0:53", "tcp://0.0.0.0:53"}
	c.Server.MaxUDPSize = utils.IPv4UdpMaxPayload

	c.Entry = []string{"", ""}

	// blackhole
	if err := c.AddPlugin("", "blackhole", blackhole.Args{
		RCode: 2,
	}); err != nil {
		return nil, err
	}

	// ecs
	if err := c.AddPlugin("", "ecs", ecs.Args{
		Auto:           false,
		ForceOverwrite: false,
		Mask4:          24,
		Mask6:          32,
		IPv4:           "1.2.3.4",
		IPv6:           "2001:dd8:1a::",
		Next:           "",
	}); err != nil {
		return nil, err
	}

	// forward
	if err := c.AddPlugin("", "forward", forward.Args{
		Upstream: []forward.Upstream{
			{"https://dns.google/dns-query", []string{"8.8.8.8", "8.8.4.4"}},
			{"https://1.1.1.1/dns-query", []string{"1.1.1.1", "1.0.0.1"}},
		},
		Timeout:            10,
		InsecureSkipVerify: false,
		Deduplicate:        false,
		Next:               "",
	}); err != nil {
		return nil, err
	}

	// ipset
	if err := c.AddPlugin("", "ipset", ipset.Args{
		SetName4: "",
		SetName6: "",
		Mask4:    24,
		Mask6:    32,
		Next:     "",
	}); err != nil {
		return nil, err
	}

	// redirect_domain
	if err := c.AddPlugin("", "redirect_domain", redirect_domain.Args{
		Domain:        []string{"", ""},
		CheckQuestion: true,
		CheckCNAME:    false,
		Redirect:      "",
		Next:          "",
	}); err != nil {
		return nil, err
	}

	// redirect_ip
	if err := c.AddPlugin("", "redirect_ip", redirect_ip.Args{
		IP:       []string{"", ""},
		Redirect: "",
		Next:     "",
	}); err != nil {
		return nil, err
	}

	// redirect_query_type
	if err := c.AddPlugin("", "redirect_query_type", redirect_query_type.Args{
		Type:     []int{1, 28},
		Redirect: "",
		Next:     "",
	}); err != nil {
		return nil, err
	}

	// sequence
	if err := c.AddPlugin("", "sequence", sequence.Args{
		Sequence: []string{"", ""},
		Next:     "",
	}); err != nil {
		return nil, err
	}

	return c, nil
}
