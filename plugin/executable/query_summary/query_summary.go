/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package query_summary

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"go.uber.org/zap"
	"time"
)

const (
	PluginType = "query_summary"
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(*Args) })
	coremain.RegNewPersetPluginFunc("_query_summary", func(bp *coremain.BP) (coremain.Plugin, error) {
		return newLogger(bp, &Args{}), nil
	})
}

var _ coremain.ExecutablePlugin = (*logger)(nil)

type Args struct {
	Msg string `yaml:"msg"`
}

func (a *Args) init() {
	if len(a.Msg) == 0 {
		a.Msg = "query summary"
	}
}

type logger struct {
	args *Args
	*coremain.BP
}

// Init is a handler.NewPluginFunc.
func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return newLogger(bp, args.(*Args)), nil
}

func newLogger(bp *coremain.BP, args *Args) coremain.Plugin {
	args.init()
	return &logger{BP: bp, args: args}
}

func (l *logger) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	err := executable_seq.ExecChainNode(ctx, qCtx, next)

	q := qCtx.Q()
	if len(q.Question) != 1 {
		return nil
	}
	question := q.Question[0]
	respRcode := -1
	if r := qCtx.R(); r != nil {
		respRcode = r.Rcode
	}

	l.BP.L().Info(
		l.args.Msg,
		zap.Uint32("uqid", qCtx.Id()),
		zap.String("qname", question.Name),
		zap.Uint16("qtype", question.Qtype),
		zap.Uint16("qclass", question.Qclass),
		zap.Stringer("client", qCtx.ReqMeta().ClientAddr),
		zap.Int("resp_rcode", respRcode),
		zap.Duration("elapsed", time.Now().Sub(qCtx.StartTime())),
		zap.Error(err),
	)
	return err
}
