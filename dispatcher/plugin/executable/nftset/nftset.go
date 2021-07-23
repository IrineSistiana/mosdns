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
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"go.uber.org/zap"
)

const PluginType = "nftset"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*nftsetPlugin)(nil)

type Args struct {
	TableName string `yaml:"table_name"`
	SetName4  string `yaml:"set_name4"`
	SetName6  string `yaml:"set_name6"`
	Mask4     uint8  `yaml:"mask4"` // default 24
	Mask6     uint8  `yaml:"mask6"` // default 32

	// max A/AAAA record ttl value.
	MaxTTL4 uint32 `yaml:"max_ttl4"`
	MaxTTL6 uint32 `yaml:"max_ttl6"`
}

type nftsetPlugin struct {
	*handler.BP
	args *Args
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newNFTSetPlugin(bp, args.(*Args)), nil
}

func newNFTSetPlugin(bp *handler.BP, args *Args) *nftsetPlugin {
	if args.Mask4 == 0 {
		args.Mask4 = 24
	}
	if args.Mask6 == 0 {
		args.Mask6 = 32
	}

	return &nftsetPlugin{
		BP:   bp,
		args: args,
	}
}

// Exec tries to add all qCtx.R() IPs to system ipset.
// If an error occurred, Exec will just log it.
// Therefore, Exec will never return an err.
func (p *nftsetPlugin) Exec(_ context.Context, qCtx *handler.Context) (_ error) {
	if qCtx.R() == nil {
		return nil
	}

	er := p.addNFTSet(qCtx.R())
	if er != nil {
		p.L().Warn("failed to add response IP to nftset", qCtx.InfoField(), zap.Error(er))
	}
	return nil
}
