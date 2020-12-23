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

package fallback

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"sync"
)

const PluginType = "fallback"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

var _ handler.Functional = (*fallback)(nil)

type fallback struct {
	args   *Args
	logger *logrus.Entry

	l      sync.RWMutex
	status []stat
	p      int
}

type stat uint8

const (
	success stat = iota
	failed
)

type Args struct {
	// Primary exec sequence, must have at least one element.
	Primary []string `yaml:"primary"`
	// Secondary exec sequence, must have at least one element.
	Secondary []string `yaml:"secondary"`

	StatLength uint `yaml:"stat_length"` // default is 10
	Threshold  uint `yaml:"threshold"`   // default is 5
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	f, err := newFallback(tag, args)
	if err != nil {
		return nil, err
	}
	return handler.WrapFunctionalPlugin(tag, PluginType, f), nil
}

func newFallback(tag string, args *Args) (*fallback, error) {
	if len(args.Primary)+len(args.Secondary) == 0 {
		return nil, fmt.Errorf("missing args: primary or secondary")
	}

	if args.Threshold > args.StatLength {
		return nil, fmt.Errorf("invalid args: threshold is bigger than stat_length")
	}

	if args.StatLength == 0 {
		args.StatLength = 10
	}

	if args.Threshold == 0 {
		args.Threshold = 5
	}

	return &fallback{
		args:   args,
		logger: mlog.NewPluginLogger(tag),
		status: make([]stat, args.StatLength),
	}, nil
}

func (f *fallback) Do(ctx context.Context, qCtx *handler.Context) (err error) {
	if f.primaryIsOk() {
		f.logger.Debugf("%v: primary is ok", qCtx)
		return f.doPrimary(ctx, qCtx)
	}
	f.logger.Debugf("%v: primary is unhealthy", qCtx)
	return f.doSecondary(ctx, qCtx)
}

func (f *fallback) doPrimary(ctx context.Context, qCtx *handler.Context) (err error) {
	err = f.do(ctx, qCtx, f.args.Primary)
	if err != nil || (qCtx.R != nil && qCtx.R.Rcode != dns.RcodeSuccess) {
		f.updatePrimaryStat(failed)
	} else {
		f.updatePrimaryStat(success)
	}
	return err
}

func (f *fallback) doSecondary(ctx context.Context, qCtx *handler.Context) (err error) {
	c := make(chan *handler.Context, 1)
	go func() {
		qCtxCopy := qCtx.Copy()
		err := f.doPrimary(ctx, qCtxCopy)
		if err == nil && qCtx.R != nil && qCtx.R.Rcode == dns.RcodeSuccess {
			select {
			case c <- qCtx:
			default:
			}
		}
		if err != nil {
			f.logger.Warnf("%v: %v", qCtx, handler.NewErrFromTemplate(handler.ETPluginErr, err))
		}
	}()

	go func() {
		err := f.do(ctx, qCtx, f.args.Secondary)
		if err == nil && qCtx.R != nil && qCtx.R.Rcode == dns.RcodeSuccess {
			select {
			case c <- qCtx:
			default:
			}
		}
		if err != nil {
			f.logger.Warnf("%v: %v", qCtx, handler.NewErrFromTemplate(handler.ETPluginErr, err))
		}
	}()

	select {
	case q := <-c:
		*qCtx = *q
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (f *fallback) do(ctx context.Context, qCtx *handler.Context, sequence []string) (err error) {
	for _, tag := range sequence {
		v, err := handler.GetPlugin(tag)
		if err != nil {
			return err
		}

		switch p := v.(type) {
		case handler.FunctionalPlugin:
			f.logger.Debugf("%v: exec functional plugin %s", qCtx, p.Tag())
			err = p.Do(ctx, qCtx)
		case handler.RouterPlugin:
			f.logger.Debugf("%v: exec router plugin %s", qCtx, p.Tag())
			err = handler.Walk(ctx, qCtx, p.Tag())
		default:
			err = fmt.Errorf("plugin %s can not be used here", p.Tag())
		}
		if err != nil {
			return handler.NewErrFromTemplate(handler.ETPluginErr, err)
		}
	}
	return nil
}

func (f *fallback) primaryIsOk() bool {
	f.l.RLock()
	defer f.l.RUnlock()
	var failedSum uint
	for _, s := range f.status {
		if s == failed {
			failedSum++
		}
	}
	return failedSum < f.args.Threshold
}

func (f *fallback) updatePrimaryStat(s stat) {
	f.l.Lock()
	defer f.l.Unlock()

	if f.p >= len(f.status) {
		f.p = 0
	}
	f.status[f.p] = s
	f.p++
}
