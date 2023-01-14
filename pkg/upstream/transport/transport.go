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
	"github.com/miekg/dns"
	"io"
	"time"
)

var (
	errEOL             = errors.New("end of life")
	errClosedTransport = errors.New("transport has been closed")
)

const (
	defaultIdleTimeout      = time.Second * 10
	defaultDialTimeout      = time.Second * 5
	defaultPipelineMaxConns = 2

	// If a pipeline connection sent a query but did not see any reply (include replies that
	// for other queries) from the server after waitingReplyTimeout. It assumes that
	// something goes wrong with the connection or the server. The connection will be closed.
	waitingReplyTimeout = time.Second * 10

	pipelineBusyQueueLen = 8
)

type IOOpts struct {
	// DialFunc specifies the method to dial a connection to the server.
	// DialFunc MUST NOT be nil.
	DialFunc func(ctx context.Context) (io.ReadWriteCloser, error)
	// WriteFunc specifies the method to write a wire dns msg to the connection
	// opened by the DialFunc.
	// WriteFunc MUST NOT be nil.
	WriteFunc func(c io.Writer, m *dns.Msg) (int, error)
	// ReadFunc specifies the method to read a wire dns msg from the connection
	// opened by the DialFunc.
	// ReadFunc MUST NOT be nil.
	ReadFunc func(c io.Reader) (*dns.Msg, int, error)

	// DialTimeout specifies the timeout for DialFunc.
	// Default is defaultDialTimeout.
	DialTimeout time.Duration

	// IdleTimeout controls the maximum idle time for each connection.
	// Default is defaultIdleTimeout.
	IdleTimeout time.Duration
}
