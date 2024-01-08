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
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"strconv"
	"strings"
)

const PluginType = "ipset"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

type Args struct {
	SetName4 string `yaml:"set_name4"`
	SetName6 string `yaml:"set_name6"`
	Mask4    int    `yaml:"mask4"`    // default 24
	Mask6    int    `yaml:"mask6"`    // default 32
	Timeout4 int    `yaml:"timeout4"` // default -1, not use
	Timeout6 int    `yaml:"timeout6"` // default -1, not use
}

var _ sequence.Executable = (*ipSetPlugin)(nil)

// QuickSetup format: [set_name,{inet|inet6},mask] *2
// e.g. "my_set,inet,24 my_set6,inet6,48"
func QuickSetup(_ sequence.BQ, s string) (any, error) {
	fs := strings.Fields(s)
	if len(fs) > 2 {
		return nil, fmt.Errorf("expect no more than 2 fields, got %d", len(fs))
	}

	args := new(Args)
	for _, argsStr := range fs {
		ss := strings.Split(argsStr, ",")
		if len(ss) != 3 && len(ss) != 4 {
			return nil, fmt.Errorf("invalid args, expect 3 or 4 fields, got %d", len(ss))
		}

		m, err := strconv.Atoi(ss[2])
		if err != nil {
			return nil, fmt.Errorf("invalid mask, %w", err)
		}
		ttl := -1
		if len(ss) == 4 {
			if ttl, err = strconv.Atoi(ss[3]); err != nil {
				return nil, fmt.Errorf("invalid timeout, %w", err)
			}
		}

		switch ss[1] {
		case "inet":
			args.Mask4 = m
			args.SetName4 = ss[0]
			args.Timeout4 = ttl
		case "inet6":
			args.Mask6 = m
			args.SetName6 = ss[0]
			args.Timeout6 = ttl
		default:
			return nil, fmt.Errorf("invalid set family, %s", ss[0])
		}
	}
	return newIpSetPlugin(args)
}
