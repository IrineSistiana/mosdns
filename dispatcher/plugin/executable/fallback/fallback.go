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

var _ handler.Executable = (*fallback)(nil)

type fallback struct {
	tag       string
	logger    *logrus.Entry
	args      *Args
	primary   *handler.ExecutableCmdSequence
	secondary *handler.ExecutableCmdSequence

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
	Primary []interface{} `yaml:"primary"`
	// Secondary exec sequence, must have at least one element.
	Secondary []interface{} `yaml:"secondary"`

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
	return handler.WrapExecutablePlugin(tag, PluginType, f), nil
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

	primary := handler.NewExecutableCmdSequence()
	if err := primary.Parse(args.Primary); err != nil {
		return nil, fmt.Errorf("invalid primary sequence: %w", err)
	}

	secondary := handler.NewExecutableCmdSequence()
	if err := secondary.Parse(args.Secondary); err != nil {
		return nil, fmt.Errorf("invalid secondary sequence: %w", err)
	}

	return &fallback{
		tag:       tag,
		logger:    mlog.NewPluginLogger(tag),
		args:      args,
		primary:   primary,
		secondary: secondary,

		status: make([]stat, args.StatLength),
	}, nil
}

func (f *fallback) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	err = f.exec(ctx, qCtx)
	if err != nil {
		return handler.NewPluginError(f.tag, err)
	}
	return nil
}

func (f *fallback) exec(ctx context.Context, qCtx *handler.Context) (err error) {
	if f.primaryIsOk() {
		f.logger.Debugf("%v: primary is ok", qCtx)
		return f.doPrimary(ctx, qCtx)
	}
	f.logger.Debugf("%v: primary is unhealthy", qCtx)
	return f.doSecondary(ctx, qCtx)
}

func (f *fallback) doPrimary(ctx context.Context, qCtx *handler.Context) (err error) {
	err = f.primary.Exec(ctx, qCtx, f.logger)
	if err != nil || qCtx.R == nil || (qCtx.R != nil && qCtx.R.Rcode != dns.RcodeSuccess) {
		f.updatePrimaryStat(failed)
	} else {
		f.updatePrimaryStat(success)
	}
	return err
}

type fallbackResult struct {
	qCtx *handler.Context
	err  error
	from string
}

func (f *fallback) doSecondary(ctx context.Context, qCtx *handler.Context) (err error) {
	c := make(chan *fallbackResult, 2) // buf size is 2, avoid block.

	go func() {
		qCtxCopy := qCtx.Copy()
		err := f.doPrimary(ctx, qCtxCopy)
		c <- &fallbackResult{
			qCtx: qCtxCopy,
			err:  err,
			from: "primary",
		}
	}()

	go func() {
		qCtxCopy := qCtx.Copy()
		err := f.secondary.Exec(ctx, qCtxCopy, f.logger)
		c <- &fallbackResult{
			qCtx: qCtxCopy,
			err:  err,
			from: "secondary",
		}
	}()

	for i := 0; i < 2; i++ {
		select {
		case r := <-c:
			if r.err != nil {
				f.logger.Warnf("%v: %s sequence failed: %v", qCtx, r.from, r.err)
			} else if r.qCtx.R == nil {
				f.logger.Debugf("%v: %s sequence returned with an empty response ", qCtx, r.from)
			} else {
				f.logger.Debugf("%v: %s sequence returned a response", qCtx, r.from)
				*qCtx = *r.qCtx
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// No response
	qCtx.SetResponse(nil, handler.ContextStatusServerFailed)
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
