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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"strings"
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
	tlsHandshakeTimeout = time.Second * 5
)

var _ handler.Executable = (*fastForward)(nil)

type fastForward struct {
	tag    string
	logger *logrus.Entry
	args   *Args

	upstream []*fastUpstream

	sfGroup utils.ExchangeSingleFlightGroup
}

type Args struct {
	Upstream    []*UpstreamConfig `yaml:"upstream"`
	Deduplicate bool              `yaml:"deduplicate"`
}

// UpstreamConfig: Note: It is not reusable.
type UpstreamConfig struct {
	// Protocol: upstream protocol, can be:
	// "", "udp" -> udp upstream
	// "tcp" -> tcp upstream
	// "dot" -> dns over tls upstream
	// "doh", "https" -> dns over https (rfc 8844) upstream
	Protocol string `yaml:"protocol"`

	// Addr: upstream network "host:port" addr, "port" can be omitted.
	// Addr can not be empty.
	Addr       string `yaml:"addr"`
	Trusted    bool   `yaml:"trusted"`     // If an upstream is "trusted", it's err rcode response will be accepted.
	Socks5     string `yaml:"socks5"`      // used by "tcp", "dot", "doh" as Socks5 server addr.
	ServerName string `yaml:"server_name"` // used by "dot" as server certificate name. It can not be empty.
	URL        string `yaml:"url"`         // used by "doh" as server endpoint url. It can not be empty.

	// Timeout: used by all protocols.
	// In "udp", "tcp", "dot", it's read timeout.
	// In "doh", it's a time limit for the query, including dial connection.
	// Default is generalReadTimeout.
	Timeout uint `yaml:"timeout"`

	// IdleTimeout used by all protocols to control connection idle timeout.
	// If IdleTimeout == 0, connection reuse will be disabled.
	IdleTimeout        uint     `yaml:"idle_timeout"`
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify"` // used by "dot", "doh". Skip tls verification.
	CA                 []string `yaml:"ca"`                   // certificate path, used by "dot", "doh" as ca root.
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	return newFastForward(tag, args)
}

func newFastForward(tag string, args *Args) (*fastForward, error) {
	if len(args.Upstream) == 0 {
		return nil, errors.New("no upstream is configured")
	}

	logger := mlog.NewPluginLogger(tag)
	f := &fastForward{
		tag:      tag,
		args:     args,
		logger:   logger,
		upstream: make([]*fastUpstream, 0),
	}

	for _, config := range args.Upstream {
		u, err := newFastUpstream(config, logger)
		if err != nil {
			return nil, err
		}
		f.upstream = append(f.upstream, u)
	}

	return f, nil
}

func (f *fastForward) Tag() string {
	return f.tag
}

func (f *fastForward) Type() string {
	return PluginType
}

// Exec forwards qCtx.Q to upstreams, and sets qCtx.R.
// qCtx.Status will be set as
// - handler.ContextStatusResponded: if it received a response.
// - handler.ContextStatusServerFailed: if all upstreams failed.
func (f *fastForward) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	err = f.exec(ctx, qCtx)
	if err != nil {
		err = handler.NewPluginError(f.tag, err)
	}
	return nil
}

func (f *fastForward) exec(ctx context.Context, qCtx *handler.Context) (err error) {
	r, err := f.exchange(ctx, qCtx)
	if err != nil {
		f.logger.Warnf("%v: forward failed: %v", qCtx, err)
		qCtx.SetResponse(nil, handler.ContextStatusServerFailed)
		return nil
	}

	qCtx.SetResponse(r, handler.ContextStatusResponded)
	return nil
}

type parallelResult struct {
	r    *dns.Msg
	err  error
	from *fastUpstream
}

func (f *fastForward) exchange(ctx context.Context, qCtx *handler.Context) (r *dns.Msg, err error) {
	if f.args.Deduplicate {
		return f.sfGroup.Exchange(ctx, qCtx, f.exchangeParallel)
	}
	return f.exchangeParallel(ctx, qCtx)
}

func (f *fastForward) exchangeParallel(ctx context.Context, qCtx *handler.Context) (r *dns.Msg, err error) {
	t := len(f.upstream)
	if t == 0 {
		return nil, errors.New("no upstream is configured")
	}
	if t == 1 {
		r, err = f.upstream[0].Exchange(qCtx.Q)
		if err != nil {
			return nil, err
		}
		return r, nil
	}

	var errs []string
	c := make(chan *parallelResult, t) // use buf chan to avoid block.
	for _, u := range f.upstream {
		u := u
		go func() {
			qCopy := qCtx.Q.Copy() // it is not safe to use the Q directly.
			r, err := u.Exchange(qCopy)
			c <- &parallelResult{
				r:    r,
				err:  err,
				from: u,
			}
		}()
	}

	for i := 0; i < t; i++ {
		select {
		case r := <-c:
			if r.err != nil {
				errs = append(errs, fmt.Sprintf("upstream %s failed: %v", r.from.Address(), r.err))
				continue
			}

			if !r.from.config.Trusted && r.r.Rcode != dns.RcodeSuccess {
				errs = append(errs, fmt.Sprintf("upstream %s responded with an err rcode: %d", r.from.Address(), r.r.Rcode))
				continue
			}

			f.logger.Debugf("%v: got response from upstream %s", qCtx, r.from.Address())
			return r.r, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// all upstreams are failed
	return nil, fmt.Errorf("all upstreams are failed: %s", strings.Join(errs, ": "))
}
