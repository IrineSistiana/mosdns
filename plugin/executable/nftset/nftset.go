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
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"strconv"
	"strings"
)

const PluginType = "nftset"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*nftSetPlugin)(nil)

type Args struct {
	IPv4 SetArgs `yaml:"ipv4"`
	IPv6 SetArgs `yaml:"ipv6"`
}

type SetArgs struct {
	TableFamily string `yaml:"table_family"`
	Table       string `yaml:"table_name"`
	Set         string `yaml:"set_name"`
	Mask        int    `yaml:"mask"`
}

// QuickSetup format: [{ip|ip6|inet},table_name,set_name,{ipv4_addr|ipv6_addr},mask] *2 (can repeat once)
// e.g. "inet,my_table,my_set,ipv4_addr,24 inet,my_table,my_set,ipv6_addr,48"
func QuickSetup(_ sequence.BQ, s string) (any, error) {
	fs := strings.Fields(s)
	if len(fs) > 2 {
		return nil, fmt.Errorf("expect no more than 2 fields, got %d", len(fs))
	}

	args := new(Args)
	for _, argsStr := range fs {
		ss := strings.Split(argsStr, ",")
		if len(ss) != 5 {
			return nil, fmt.Errorf("invalid args, expect 5 fields, got %d", len(ss))
		}

		m, err := strconv.Atoi(ss[4])
		if err != nil {
			return nil, fmt.Errorf("invalid mask, %w", err)
		}
		sa := SetArgs{
			TableFamily: ss[0],
			Table:       ss[1],
			Set:         ss[2],
			Mask:        m,
		}
		switch ss[3] {
		case "ipv4_addr":
			args.IPv4 = sa
		case "ipv6_addr":
			args.IPv6 = sa
		default:
			return nil, fmt.Errorf("invalid ip type, %s", ss[0])
		}
	}
	return newNftSetPlugin(args)
}
