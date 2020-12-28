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

package parallel

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/sirupsen/logrus"
)

const PluginType = "parallel"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

var _ handler.Executable = (*parallel)(nil)

type parallel struct {
	tag    string
	logger *logrus.Entry

	sequence []*handler.ExecutableCmdSequence
}

func (p *parallel) Tag() string {
	return p.tag
}

func (p *parallel) Type() string {
	return PluginType
}

type Args struct {
	Exec [][]interface{} `yaml:"exec"`
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	return newParallel(tag, args)
}

func newParallel(tag string, args *Args) (*parallel, error) {
	if len(args.Exec) == 0 {
		return nil, errors.New("empty sequence")
	}

	ps := make([]*handler.ExecutableCmdSequence, 0)
	for i, subSequence := range args.Exec {
		if len(subSequence) == 0 {
			return nil, fmt.Errorf("parallel sequence at index %d is empty", i)
		}

		es := handler.NewExecutableCmdSequence()
		if err := es.Parse(subSequence); err != nil {
			return nil, fmt.Errorf("invalid parallel sequence at index %d: %w", i, err)
		}
		ps = append(ps, es)
	}

	return &parallel{
		tag:      tag,
		logger:   mlog.NewPluginLogger(tag),
		sequence: ps,
	}, nil
}

func (p *parallel) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	err = p.exec(ctx, qCtx)
	if err != nil {
		return handler.NewPluginError(p.tag, err)
	}
	return nil
}

type parallelResult struct {
	qCtx *handler.Context
	err  error
	from int
}

func (p *parallel) exec(ctx context.Context, qCtx *handler.Context) (err error) {
	t := len(p.sequence)
	if t == 1 {
		return p.sequence[0].Exec(ctx, qCtx, p.logger)
	}

	c := make(chan *parallelResult, t) // use buf chan to avoid block.
	for i, sequence := range p.sequence {
		i := i
		sequence := sequence
		go func() {
			qCtxCopy := qCtx.Copy()
			err := sequence.Exec(ctx, qCtxCopy, p.logger)
			c <- &parallelResult{
				qCtx: qCtxCopy,
				err:  err,
				from: i,
			}
		}()
	}

	for i := 0; i < t; i++ {
		select {
		case r := <-c:
			if r.err != nil {
				p.logger.Warnf("%v: parallel sequence %d failed with err: %v", qCtx, r.from, r.err)
			} else if r.qCtx.Status != handler.ContextStatusResponded {
				p.logger.Debugf("%v: parallel sequence %d returned with status %s", qCtx, r.from, r.qCtx.Status)
			} else {
				p.logger.Debugf("%v: parallel sequence %d returned a good response", qCtx, r.from)
				*qCtx = *r.qCtx
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// No valid respond, all parallel sequences failed. Set qCtx.R with dns.RcodeServerFailure.
	qCtx.SetResponse(nil, handler.ContextStatusServerFailed)
	return nil
}
