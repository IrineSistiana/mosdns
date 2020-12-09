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

package fastforward

import (
	"context"
	"errors"
	"fmt"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"time"
)

const PluginType = "fast_forward"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

const (
	dialTimeout         = time.Second * 5
	generalWriteTimeout = time.Second * 1
	generalReadTimeout  = time.Second * 5
	tcpIdleTimeout      = time.Second * 5
)

var _ handler.Functional = (*fastForward)(nil)

type fastForward struct {
	upstream []upstream.Upstream
	logger   *logrus.Entry
}

type Args struct {
	Upstream []string `yaml:"upstream"`
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	f := new(fastForward)
	f.logger = mlog.NewPluginLogger(tag)
	for _, addr := range args.Upstream {
		host, preferTCP, err := parseAddr(addr)
		if err != nil {
			return nil, err
		}
		f.upstream = append(f.upstream, newFastUpstream(host, preferTCP))
	}
	return handler.WrapFunctionalPlugin(tag, PluginType, f), nil
}

func parseAddr(addr string) (host string, preferTCP bool, err error) {
	var protocol string
	protocol, host = utils.ParseAddr(addr)

	switch protocol {
	case "", "udp":
		preferTCP = false
	case "tcp":
		preferTCP = true
	default:
		err = fmt.Errorf("unsupported protocol: %s", protocol)
		return "", false, err
	}
	host = utils.TryAddPort(host, 53)
	return
}

// Do forwards qCtx.Q to upstreams, and sets qCtx.R.
// If qCtx.Q is nil, or upstreams failed, qCtx.R will be a simple response
// with RCODE = 2.
func (f *fastForward) Do(_ context.Context, qCtx *handler.Context) (err error) {
	if qCtx == nil {
		return
	}

	if qCtx.Q == nil {
		return errors.New("invalid qCtx, qCtx.Q is nil")
	}

	r, err := f.forward(qCtx.Q)
	if err != nil {
		f.logger.Warnf("%v: upstream failed: %v", qCtx, err)
		r = new(dns.Msg)
		r.SetReply(qCtx.Q)
		r.Rcode = dns.RcodeServerFailure
		qCtx.R = r
		return nil
	}
	qCtx.R = r
	return nil
}

func (f *fastForward) forward(q *dns.Msg) (r *dns.Msg, err error) {
	r, _, err = upstream.ExchangeParallel(f.upstream, q)
	return r, err
}
