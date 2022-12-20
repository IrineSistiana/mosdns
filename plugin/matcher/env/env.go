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

package env

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"os"
	"strings"
)

const PluginType = "env"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

func QuickSetup(_ sequence.BQ, s string) (sequence.Matcher, error) {
	ss := strings.Fields(s)
	var k, v string
	switch len(ss) {
	case 1:
		k = ss[0]
	case 2:
		k = ss[0]
		k = ss[1]
	default:
		return nil, fmt.Errorf("invalid arg number %d", len(ss))
	}
	return CheckEnv(k, v), nil
}

// CheckEnv checks if k is in env. If v is given, it checks whether env["k"] == v.
func CheckEnv(k, v string) sequence.Matcher {
	var res bool
	e, ok := os.LookupEnv(k)
	if ok {
		if len(v) == 0 {
			res = true
		} else {
			res = e == v
		}
	} else {
		res = false
	}

	if res {
		return sequence.MatchAlwaysTrue{}
	}
	return sequence.MatchAlwaysFalse{}
}
