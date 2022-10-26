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
	coremain.RegNewPersetPluginFunc("_query_summary", func(bp *coremain.BP) (coremain.Plugin, error) {
		return newLogger(bp), nil
	})
}

var _ coremain.ExecutablePlugin = (*logger)(nil)

type logger struct {
	*coremain.BP
}

func newLogger(bp *coremain.BP) coremain.Plugin { return &logger{BP: bp} }

func (t *logger) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	if err := executable_seq.ExecChainNode(ctx, qCtx, next); err != nil {
		return err
	}

	q := qCtx.Q()
	if len(q.Question) != 1 {
		return nil
	}
	question := q.Question[0]
	respRcode := -1
	if r := qCtx.R(); r != nil {
		respRcode = r.Rcode
	}

	logger := t.BP.L()
	logger.Info("query summary",
		zap.Uint32("uqid", qCtx.Id()),
		zap.String("qname", question.Name),
		zap.Uint16("qtype", question.Qtype),
		zap.Uint16("qclass", question.Qclass),
		zap.Stringer("client", qCtx.ReqMeta().ClientAddr),
		zap.Int("resp_rcode", respRcode),
		zap.Duration("elapsed", time.Now().Sub(qCtx.StartTime())),
	)
	return nil
}
