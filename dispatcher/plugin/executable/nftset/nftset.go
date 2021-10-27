//     Copyright (C) 2020-2021, IrineSistiana
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

package nftset

import (
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
)

const PluginType = "nftset"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*nftsetPlugin)(nil)

type Args struct {
	TableFamily4 string `yaml:"table_family4"`
	TableFamily6 string `yaml:"table_family6"`
	TableName4   string `yaml:"table_name4"`
	TableName6   string `yaml:"table_name6"`
	SetName4     string `yaml:"set_name4"`
	SetName6     string `yaml:"set_name6"`
	Mask4        int    `yaml:"mask4"` // default 24
	Mask6        int    `yaml:"mask6"` // default 32
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newNftsetPlugin(bp, args.(*Args))
}
