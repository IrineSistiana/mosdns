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

	handler.MustRegPlugin(handler.WrapExecutablePlugin("_drop_response", PluginType, &blackhole{rCode: -1}))
	handler.MustRegPlugin(handler.WrapExecutablePlugin("_block_with_servfail", PluginType, &blackhole{rCode: dns.RcodeServerFailure}))
	handler.MustRegPlugin(handler.WrapExecutablePlugin("_block_with_nxdomain", PluginType, &blackhole{rCode: dns.RcodeNameError}))
}

var _ handler.Executable = (*blackhole)(nil)

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

	return handler.WrapExecutablePlugin(tag, PluginType, b), nil
}

// Do drops or replaces qCtx.R with a simple denial response.
// It never returns an err.
func (b *blackhole) Exec(_ context.Context, qCtx *handler.Context) (err error) {
	if qCtx == nil || qCtx.Q == nil {
		return nil
	}

	switch {
	case b.rCode >= 0:
		r := new(dns.Msg)
		r.SetReply(qCtx.Q)
		r.Rcode = b.rCode
		qCtx.R = r
	default:
		qCtx.R = nil
	}

	return nil
}
