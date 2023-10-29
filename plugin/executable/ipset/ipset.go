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
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
)

const PluginType = "ipset"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	SetName4 string `yaml:"set_name4"`
	SetName6 string `yaml:"set_name6"`
	Mask4    int    `yaml:"mask4"` // default 24
	Mask6    int    `yaml:"mask6"` // default 32
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newIpsetPlugin(bp, args.(*Args))
}
