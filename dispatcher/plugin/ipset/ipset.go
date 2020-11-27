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

package ipset

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
)

func init() {
	handler.RegInitFunc("ipset", Init)
}

type Args struct {
	SetName4 string `yaml:"set_name4"`
	SetName6 string `yaml:"set_name6"`
	Mask4    int    `yaml:"mask4"`
	Mask6    int    `yaml:"mask6"`

	Next string `yaml:"next"`
}

type ipsetPlugin struct {
	setName4, setName6 string
	mask4, mask6       uint8
}

func Init(conf *handler.Config) (p handler.Plugin, err error) {
	args := new(Args)
	err = conf.Args.WeakDecode(args)
	if err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	ipsetPlugin := new(ipsetPlugin)

	ipsetPlugin.setName4 = args.SetName4
	ipsetPlugin.setName6 = args.SetName6
	ipsetPlugin.mask4 = uint8(args.Mask4)
	ipsetPlugin.mask6 = uint8(args.Mask6)

	return handler.WrapOneWayPlugin(conf, ipsetPlugin, args.Next), nil
}

func (p *ipsetPlugin) Modify(ctx context.Context, qCtx *handler.Context) (err error) {
	if qCtx == nil || qCtx.R == nil {
		return errors.New("invalid qCtx, R is nil")
	}

	return p.addIPSet(qCtx.R)
}
