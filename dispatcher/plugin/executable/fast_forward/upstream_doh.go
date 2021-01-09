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

package fastforward

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"io"
	"net/http"
	"sync"
)

func (u *fastUpstream) exchangeDoH(q *dns.Msg) (r *dns.Msg, err error) {

	buf, err := utils.GetMsgBufFor(q)
	if err != nil {
		return nil, err
	}
	defer utils.ReleaseMsgBuf(buf)

	rRaw, err := q.PackBuffer(buf)
	if err != nil {
		return nil, fmt.Errorf("invalid msg: %w", err)
	}

	// In order to maximize HTTP cache friendliness, DoH clients using media
	// formats that include the ID field from the DNS message header, such
	// as "application/dns-message", SHOULD use a DNS ID of 0 in every DNS
	// request.
	// https://tools.ietf.org/html/rfc8484#section-4.1
	rRaw[0] = 0
	rRaw[1] = 0

	urlBuilder := acquireURLBuilder()
	defer releaseURLBuilder(urlBuilder)

	// Padding characters for base64url MUST NOT be included.
	// See: https://tools.ietf.org/html/rfc8484#section-6.
	// That's why we use base64.RawURLEncoding.
	urlBuilder.Grow(len(u.config.URL) + base64.RawURLEncoding.EncodedLen(len(rRaw)))
	urlBuilder.WriteString(u.config.URL)
	urlBuilder.WriteString("?dns=")
	encoder := base64.NewEncoder(base64.RawURLEncoding, urlBuilder)
	encoder.Write(rRaw)
	encoder.Close()

	ctx, cancel := context.WithTimeout(context.Background(), u.timeout)
	defer cancel()

	r, err = u.doHTTP(ctx, urlBuilder.String())
	if err != nil {
		return nil, err
	}

	if r.Id != 0 { // check msg id
		return nil, dns.ErrId
	}
	// change the id back
	r.Id = q.Id
	return r, nil
}

func (u *fastUpstream) doHTTP(ctx context.Context, url string) (*dns.Msg, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("interal err: NewRequestWithContext: %w", err)
	}

	req.Header["Accept"] = []string{"application/dns-message"}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	// check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad http status codes %d", resp.StatusCode)
	}

	bb := acquireReadBuf()
	defer releaseReadBuf(bb)
	_, err = bb.ReadFrom(io.LimitReader(resp.Body, dns.MaxMsgSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read http body: %w", err)
	}

	r := new(dns.Msg)
	if err := r.Unpack(bb.Bytes()); err != nil {
		return nil, fmt.Errorf("invalid reply: %w", err)
	}
	return r, nil
}

var (
	bytesBufPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	stringBuilderPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
)

func acquireReadBuf() *bytes.Buffer {
	return bytesBufPool.Get().(*bytes.Buffer)
}

func releaseReadBuf(buf *bytes.Buffer) {
	buf.Reset()
	bytesBufPool.Put(buf)
}

func acquireURLBuilder() *bytes.Buffer {
	return stringBuilderPool.Get().(*bytes.Buffer)
}

func releaseURLBuilder(builder *bytes.Buffer) {
	builder.Reset()
	stringBuilderPool.Put(builder)
}
