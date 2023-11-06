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

package transport

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrClosedTransport                     = errors.New("transport has been closed")
	ErrPayloadOverFlow                     = errors.New("payload is too large")
	ErrNewConnCannotReserveQueryExchanger  = errors.New("new connection failed to reserve query exchanger")
	ErrLazyConnCannotReserveQueryExchanger = errors.New("lazy connection failed to reserve query exchanger")
)

const (
	defaultIdleTimeout = time.Second * 10
	defaultDialTimeout = time.Second * 5

	// If a pipeline connection sent a query but did not see any reply (include replies that
	// for other queries) from the server after waitingReplyTimeout. It assumes that
	// something goes wrong with the connection or the server. The connection will be closed.
	waitingReplyTimeout = time.Second * 10

	defaultTdcMaxConcurrentQuery = 32
	defaultMaxLazyConnQueue      = 16
)

// One method MUST be called in ReservedExchanger.
type ReservedExchanger interface {
	// ExchangeReserved sends q to the server and returns it's response.
	// ExchangeReserved MUST not modify nor keep the q.
	// q MUST be a valid dns message.
	// resp (if no err) should be released by ReleaseResp().
	ExchangeReserved(ctx context.Context, q []byte) (resp *[]byte, err error)

	// WithdrawReserved aborts the query.
	WithdrawReserved()
}

type DnsConn interface {
	// ReserveNewQuery reserves a query. It MUST be fast and non-block. If DnsConn
	// cannot serve more query due to its capacity, ReserveNewQuery returns nil.
	// If DnsConn is closed and can no longer serve more query, returns closed = true.
	ReserveNewQuery() (_ ReservedExchanger, closed bool)
	io.Closer
}

type NetConn interface {
	io.ReadWriteCloser
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}
