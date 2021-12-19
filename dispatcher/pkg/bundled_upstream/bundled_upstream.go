//     Copyright (C) 2020-2021, IrineSistiana
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

package bundled_upstream

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type Upstream interface {
	Exchange(ctx context.Context, q *dns.Msg) (*dns.Msg, error)
	Address() string
	Trusted() bool
}

type BundledUpstream struct {
	us     []Upstream
	logger *zap.Logger
}

// NewBundledUpstream creates a BundledUpstream.
// us must contain at least one Upstream.
// To disable logger, set it to nil.
func NewBundledUpstream(us []Upstream, logger *zap.Logger) *BundledUpstream {
	if len(us) < 1 {
		panic("us must contain at least one Upstream")
	}

	if logger == nil {
		logger = zap.NewNop()
	}
	return &BundledUpstream{us: us, logger: logger}
}

type parallelResult struct {
	r    *dns.Msg
	err  error
	from Upstream
}

func (bu *BundledUpstream) ExchangeParallel(ctx context.Context, qCtx *handler.Context) (*dns.Msg, error) {
	q := qCtx.Q()
	t := len(bu.us)
	if t == 1 {
		u := bu.us[0]
		r, err := u.Exchange(ctx, q)
		if err != nil {
			return nil, err
		}
		bu.logger.Debug("response received", qCtx.InfoField(), zap.String("from", u.Address()))
		return r, nil
	}

	c := make(chan *parallelResult, t) // use buf chan to avoid blocking.
	qCopy := q.Copy()                  // qCtx is not safe for concurrent use.
	for _, u := range bu.us {
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

	var candidateErrReply *parallelResult
	for i := 0; i < t; i++ {
		select {
		case res := <-c:
			if res.err != nil {
				bu.logger.Warn("upstream failed", qCtx.InfoField(), zap.String("from", res.from.Address()), zap.Error(res.err))
				continue
			}

			if res.r.Rcode == dns.RcodeSuccess || res.from.Trusted() {
				bu.logger.Debug("response accepted", qCtx.InfoField(), zap.String("from", res.from.Address()))
				return res.r, nil
			}

			if candidateErrReply == nil {
				candidateErrReply = res
			}
			bu.logger.Debug("untrusted upstream returned an err rcode", qCtx.InfoField(), zap.String("from", res.from.Address()), zap.Int("rcode", res.r.Rcode))
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// all upstreams failed or returned a error rcode
	if candidateErrReply != nil {
		bu.logger.Debug("candidate error response accepted", qCtx.InfoField(), zap.String("from", candidateErrReply.from.Address()))
		return candidateErrReply.r, nil
	}

	return nil, errors.New("no response")
}
