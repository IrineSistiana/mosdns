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

package server

import (
	"crypto/tls"
	"errors"
	"github.com/IrineSistiana/mosdns/v4/pkg/server/dns_handler"
	"go.uber.org/zap"
	"io"
	"net/http"
	"sync"
	"time"
)

var (
	ErrServerClosed = errors.New("server closed")

	nopLogger = zap.NewNop()
)

// Server is a DNS server.
// It's functions, Server.ServeUDP etc., will block and
// close the net.Listener/net.PacketConn and always return
// a non-nil error. If Server was closed, the returned err
// will be ErrServerClosed.
type Server struct {
	// DNSHandler is the dns handler required by UDP, TCP, DoT server.
	DNSHandler dns_handler.Handler

	// HttpHandler is the http handler required by HTTP, DoH server.
	HttpHandler http.Handler

	// TLSConfig is required by DoT, DoH server.
	// It must contain at least one certificate. If not, caller should use
	// Cert, Key to load a certificate from disk.
	TLSConfig *tls.Config

	// Certificate files to start DoT, DoH server.
	// Only useful if there is no server certificate specified in TLSConfig.
	Cert, Key string

	// IdleTimeout limits the maximum time period that a connection
	// can idle. Default is defaultIdleTimeout.
	IdleTimeout time.Duration

	// Logger optionally specifies logger for the server logging.
	// A nil Logger will disables the logging.
	Logger *zap.Logger

	m             sync.Mutex
	closed        bool
	closerTracker map[*io.Closer]struct{}
}

// getLogger always returns a non-nil logger.
func (s *Server) getLogger() *zap.Logger {
	if l := s.Logger; l != nil {
		return l
	}
	return nopLogger
}

func (s *Server) getIdleTimeout() time.Duration {
	if t := s.IdleTimeout; t > 0 {
		return t
	}
	return defaultTCPIdleTimeout
}

// Closed returns true if server was closed.
func (s *Server) Closed() bool {
	s.m.Lock()
	defer s.m.Unlock()
	return s.closed
}

// trackCloser adds or removes c to the Server and return true if Server is not closed.
// We use a pointer in case the underlying value is incomparable.
func (s *Server) trackCloser(c *io.Closer, add bool) bool {
	s.m.Lock()
	defer s.m.Unlock()

	if s.closerTracker == nil {
		s.closerTracker = make(map[*io.Closer]struct{})
	}

	if add {
		if s.closed {
			return false
		}
		s.closerTracker[c] = struct{}{}
	} else {
		delete(s.closerTracker, c)
	}
	return true
}

// Close closes the Server and all its inner listeners.
func (s *Server) Close() {
	s.m.Lock()
	defer s.m.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	for closer := range s.closerTracker {
		(*closer).Close()
	}
	return
}
