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

package forward

import (
	"context"
	"errors"
	"fmt"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"golang.org/x/sync/singleflight"
	"net"
	"time"
)

const PluginType = "forward"

func init() {
	handler.RegInitFunc(PluginType, Init)
	handler.SetTemArgs(
		PluginType,
		&Args{
			Upstream: []Upstream{
				{Addr: "https://dns.google/dns-query", IPAddr: []string{"8.8.8.8", "8.8.4.4", "2001:4860:4860::8888", "2001:4860:4860::8844"}},
				{Addr: "https://cloudflare-dns.com/dns-query"},
			},
			Bootstrap:          []string{"https://223.5.5.5/dns-query", "https://1.1.1.1/dns-query"},
			Timeout:            10,
			InsecureSkipVerify: false,
			Deduplicate:        false,
		},
	)

}

var _ handler.Functional = (*forwarder)(nil)

type forwarder struct {
	upstream []upstream.Upstream

	deduplicate bool
	sfGroup     singleflight.Group
}

type Args struct {
	// options for dnsproxy upstream
	Upstream           []Upstream `yaml:"upstream"`
	Timeout            int        `yaml:"timeout"`
	InsecureSkipVerify bool       `yaml:"insecure_skip_verify"`
	Bootstrap          []string   `yaml:"bootstrap"`

	// options for mosdns
	Deduplicate bool `yaml:"deduplicate"`
}

type Upstream struct {
	Addr   string   `yaml:"addr"`
	IPAddr []string `yaml:"ip_addr"`
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	f := new(forwarder)

	for _, u := range args.Upstream {
		if len(u.Addr) == 0 {
			return nil, errors.New("missing upstream address")
		}

		serverIPAddrs := make([]net.IP, 0, len(u.IPAddr))
		for _, s := range u.IPAddr {
			ip := net.ParseIP(s)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip addr %s", s)
			}
			serverIPAddrs = append(serverIPAddrs, ip)
		}

		opt := upstream.Options{}
		opt.Bootstrap = args.Bootstrap
		opt.ServerIPAddrs = serverIPAddrs

		if args.Timeout <= 0 {
			opt.Timeout = time.Second * 10
		} else {
			opt.Timeout = time.Second * time.Duration(args.Timeout)
		}

		opt.InsecureSkipVerify = args.InsecureSkipVerify

		u, err := upstream.AddressToUpstream(u.Addr, opt)
		if err != nil {
			return nil, fmt.Errorf("failed to init upsteam: %w", err)
		}

		f.upstream = append(f.upstream, u)
	}

	if len(f.upstream) == 0 {
		return nil, errors.New("missing upstream")
	}
	f.deduplicate = args.Deduplicate

	return handler.WrapFunctionalPlugin(tag, PluginType, f), nil
}

// Do forwards qCtx.Q to upstreams, and sets qCtx.R.
// If qCtx.Q is nil, or upstreams failed, qCtx.R will be a simple response
// with RCODE = 2.
func (f *forwarder) Do(_ context.Context, qCtx *handler.Context) (err error) {
	if qCtx == nil {
		return
	}

	var r *dns.Msg
	if qCtx.Q == nil {
		goto whenErr
	}

	if f.deduplicate {
		r, err = f.forwardSingleFlight(qCtx.Q)
	} else {
		r, err = f.forward(qCtx.Q)
	}

	if err == nil {
		qCtx.R = r
		return nil
	}

whenErr:
	r = new(dns.Msg)
	r.SetReply(qCtx.Q)
	r.Rcode = dns.RcodeServerFailure
	qCtx.R = r
	return nil
}

func (f *forwarder) forward(q *dns.Msg) (r *dns.Msg, err error) {
	r, _, err = upstream.ExchangeParallel(f.upstream, q)
	return r, err
}

func (f *forwarder) forwardSingleFlight(q *dns.Msg) (r *dns.Msg, err error) {
	key, err := getMsgKey(q)
	if err != nil {
		return nil, fmt.Errorf("failed to caculate msg key, %w", err)
	}

	v, err, shared := f.sfGroup.Do(key, func() (interface{}, error) {
		defer f.sfGroup.Forget(key)
		return f.forward(q)
	})

	if err != nil {
		return nil, err
	}

	rUnsafe := v.(*dns.Msg)

	if shared && rUnsafe != nil { // shared reply may has different id and is not safe to modify.
		r = rUnsafe.Copy()
		r.Id = q.Id
		return r, nil
	}

	return rUnsafe, nil
}

func getMsgKey(m *dns.Msg) (string, error) {
	buf, err := utils.GetMsgBufFor(m)
	if err != nil {
		return "", err
	}
	defer utils.ReleaseMsgBuf(buf)

	wireMsg, err := m.PackBuffer(buf)
	if err != nil {
		return "", err
	}

	wireMsg[0] = 0
	wireMsg[1] = 1
	return string(wireMsg), nil
}
