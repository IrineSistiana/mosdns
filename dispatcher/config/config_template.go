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
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/functional/blackhole"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/functional/ecs"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/functional/forward"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/functional/ipset"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/matcher/domain_matcher"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/matcher/ip_matcher"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/matcher/qtype_matcher"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/router/sequence"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
)

func GetTemplateConfig() (*Config, error) {
	c := new(Config)
	c.Server.Bind = []string{
		"udp://127.0.0.1:53",
		"tcp://127.0.0.1:53",
		"udp://[::1]:53",
		"tcp://[::1]:53",
	}
	c.Server.MaxUDPSize = utils.IPv4UdpMaxPayload

	c.Plugin.Entry = []string{"", ""}

	// blackhole
	if err := AddPlugin(&c.Plugin.Functional, "", blackhole.PluginType, &blackhole.Args{
		RCode: 2,
	}); err != nil {
		return nil, err
	}

	// ecs
	if err := AddPlugin(&c.Plugin.Functional, "", "ecs", &ecs.Args{}); err != nil {
		return nil, err
	}

	// forward
	if err := AddPlugin(&c.Plugin.Functional, "", forward.PluginType, &forward.Args{
		Upstream: []forward.Upstream{
			{"", []string{"", ""}},
			{"", []string{"", ""}},
		},
		Timeout:            10,
		InsecureSkipVerify: false,
		Deduplicate:        false,
	}); err != nil {
		return nil, err
	}

	// ipset
	if err := AddPlugin(&c.Plugin.Functional, "", ipset.PluginType, &ipset.Args{}); err != nil {
		return nil, err
	}

	// domain_matcher
	if err := AddPlugin(&c.Plugin.Matcher, "", domainmatcher.PluginType, &domainmatcher.Args{
		Domain: []string{"", ""},
	}); err != nil {
		return nil, err
	}

	// ip_matcher
	if err := AddPlugin(&c.Plugin.Matcher, "", ipmatcher.PluginType, &ipmatcher.Args{
		IP: []string{"", ""},
	}); err != nil {
		return nil, err
	}

	// qtype_matcher
	if err := AddPlugin(&c.Plugin.Matcher, "", qtypematcher.PluginType, &qtypematcher.Args{
		Type: []int{1, 28},
	}); err != nil {
		return nil, err
	}

	// sequence
	if err := AddPlugin(&c.Plugin.Router, "", sequence.PluginType, &sequence.Args{
		Sequence: []*sequence.Block{{
			If:       "",
			Exec:     []string{"", ""},
			Sequence: []*sequence.Block{{}, {}},
			Goto:     "",
		}, {}},
		Next: "",
	}); err != nil {
		return nil, err
	}
	return c, nil
}
