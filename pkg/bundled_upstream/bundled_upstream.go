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

package bundled_upstream

import (
	"context"
	"errors"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type Upstream interface {
	// Exchange sends q to the upstream and waits for response.
	// If any error occurs. Implements must return a nil msg with a non nil error.
	// Otherwise, Implements must a msg with nil error.
	Exchange(ctx context.Context, q *dns.Msg) (*dns.Msg, error)

	// Trusted indicates whether this Upstream is trusted/reliable.
	// If true, responses from this Upstream will be accepted without checking its rcode.
	Trusted() bool

	Address() string
}

type parallelResult struct {
	r    *dns.Msg
	err  error
	from Upstream
}

var nopLogger = zap.NewNop()

var ErrAllFailed = errors.New("all upstreams failed")

func ExchangeParallel(ctx context.Context, qCtx *query_context.Context, upstreams []Upstream, logger *zap.Logger) (*dns.Msg, error) {
	if logger == nil {
		logger = nopLogger
	}

	q := qCtx.Q()
	t := len(upstreams)
	if t == 1 {
		return upstreams[0].Exchange(ctx, q)
	}

	c := make(chan *parallelResult, t) // use buf chan to avoid blocking.
	qCopy := q.Copy()                  // qCtx is not safe for concurrent use.
	for _, u := range upstreams {
		u := u
		go func() {
			r, err := u.Exchange(ctx, qCopy)
			c <- &parallelResult{
				r:    r,
				err:  err,
				from: u,
			}
		}()
	}

	for i := 0; i < t; i++ {
		select {
		case res := <-c:
			if res.err != nil {
				logger.Warn("upstream err", qCtx.InfoField(), zap.String("addr", res.from.Address()))
				continue
			}

			if res.r == nil {
				continue
			}

			if res.from.Trusted() || res.r.Rcode == dns.RcodeSuccess {
				return res.r, nil
			}
			continue

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, ErrAllFailed
}
