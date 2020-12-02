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

package blackhole

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
)

const PluginType = "blackhole"

func init() {
	handler.RegInitFunc(PluginType, Init)
	handler.SetTemArgs(PluginType, &Args{})
}

var _ handler.Functional = (*blackhole)(nil)

type blackhole struct {
	rCode int
}

type Args struct {
	RCode int `yaml:"rcode"`
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	b := new(blackhole)
	b.rCode = args.RCode

	return handler.WrapFunctionalPlugin(tag, PluginType, b), nil
}

// Modify drops or replaces qCtx.R.
// It never returns a err.
func (b *blackhole) Do(_ context.Context, qCtx *handler.Context) (err error) {
	if qCtx == nil {
		return nil
	}
	if b.rCode != dns.RcodeSuccess && qCtx.Q != nil {
		r := new(dns.Msg)
		r.SetReply(qCtx.Q)
		r.Rcode = b.rCode
		qCtx.R = r
	} else {
		qCtx.R = nil
	}
	return nil
}
