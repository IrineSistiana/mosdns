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

package doh

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const (
	defaultDoHTimeout = time.Second * 6
)

var nopLogger = zap.NewNop()

// Upstream is a DNS-over-HTTPS (RFC 8484) upstream.
type Upstream struct {
	rt          http.RoundTripper
	logger      *zap.Logger // non-nil
	urlTemplate *urlpkg.URL
	reqTemplate *http.Request
}

func NewUpstream(endPoint string, rt http.RoundTripper, logger *zap.Logger) (*Upstream, error) {
	req, err := http.NewRequest(http.MethodGet, endPoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse http request, %w", err)
	}

	req.Header["Accept"] = []string{"application/dns-message"}
	req.Header["User-Agent"] = nil // Don't let go http send a default user agent header.

	if logger == nil {
		logger = nopLogger
	}
	return &Upstream{
		rt:          rt,
		logger:      logger,
		urlTemplate: req.URL,
		reqTemplate: req,
	}, nil
}

var (
	bufPool4k = pool.NewBytesBufPool(4096)
)

func (u *Upstream) ExchangeContext(ctx context.Context, q []byte) (*[]byte, error) {
	bp := pool.GetBuf(len(q))
	defer pool.ReleaseBuf(bp)
	wire := *bp
	copy(wire, q)

	// In order to maximize HTTP cache friendliness, DoH clients using media
	// formats that include the ID field from the DNS message header, such
	// as "application/dns-message", SHOULD use a DNS ID of 0 in every DNS
	// request.
	// https://tools.ietf.org/html/rfc8484#section-4.1
	wire[0] = 0
	wire[1] = 0

	queryLen := 4 + base64.RawURLEncoding.EncodedLen(len(wire))
	queryBuf := make([]byte, queryLen)

	p := 0
	p += copy(queryBuf, "dns=")

	// Padding characters for base64url MUST NOT be included.
	// See: https://tools.ietf.org/html/rfc8484#section-6.
	base64.RawURLEncoding.Encode(queryBuf[p:], wire)

	type res struct {
		r   *[]byte
		err error
	}

	resChan := make(chan res, 1)
	go func() {
		// We overwrite the ctx with a fixed timeout context here.
		// Because the http package may close the underlay connection
		// if the context is done before the query is completed. This
		// reduces the connection reuse efficiency.
		ctx, cancel := context.WithTimeout(context.Background(), defaultDoHTimeout)
		defer cancel()
		r, err := u.exchange(ctx, utils.BytesToStringUnsafe(queryBuf))
		if err != nil {
			u.logger.Check(zap.WarnLevel, "exchange failed").Write(zap.Error(err))
		}
		resChan <- res{r: r, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case res := <-resChan:
		r := res.r
		err := res.err
		if r != nil {
			binary.BigEndian.PutUint16(*r, binary.BigEndian.Uint16(q))
		}
		return r, err
	}
}

func (u *Upstream) exchange(ctx context.Context, dnsQuery string) (*[]byte, error) {
	req := u.reqTemplate.WithContext(ctx)
	req.URL = new(urlpkg.URL)
	*req.URL = *u.urlTemplate
	req.URL.RawQuery = dnsQuery
	resp, err := u.rt.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	// check status code
	if resp.StatusCode != http.StatusOK {
		body1k, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if body1k != nil {
			return nil, fmt.Errorf("bad http status codes %d with body [%s]", resp.StatusCode, body1k)
		}
		return nil, fmt.Errorf("bad http status codes %d", resp.StatusCode)
	}

	bb := bufPool4k.Get()
	defer bufPool4k.Release(bb)
	_, err = bb.ReadFrom(io.LimitReader(resp.Body, dns.MaxMsgSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read http body: %w", err)
	}
	if bb.Len() < dnsutils.DnsHeaderLen {
		return nil, dnsutils.ErrPayloadTooSmall
	}
	payload := pool.GetBuf(bb.Len())
	copy(*payload, bb.Bytes())
	return payload, nil
}
