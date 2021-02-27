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

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"go.uber.org/zap"
	"net"
	"sync"
	"time"
)

// Server errors
var (
	ErrServerClosed  = errors.New("server closed")
	ErrServerStarted = errors.New("server has been started")
	errServerExited  = errors.New("server exited without err")
)

type Protocol uint8

// DNS protocols
const (
	ProtocolUDP Protocol = iota
	ProtocolTCP
	ProtocolDoT
	ProtocolDoH
	ProtocolHttp
)

const (
	defaultQueryTimeout = time.Second * 5
	defaultIdleTimeout  = time.Second * 10
)

type Server struct {
	Handler  DNSServerHandler
	Protocol Protocol

	Listener   net.Listener
	PacketConn net.PacketConn

	TLSConfig *tls.Config // Used by dot, doh in tls.NewListener.

	URLPath             string // Used by doh, http. If it's emtpy, any path will be handled.
	GetUserIPFromHeader string // Used by doh, http. Get user ip address from http header.

	QueryTimeout time.Duration // Used by all protocol as query timeout, default is defaultQueryTimeout.
	IdleTimeout  time.Duration // Used by tcp, dot, doh as connection idle timeout, default is defaultIdleTimeout.

	Logger *zap.Logger // Nil logger disables logging.

	mu      sync.Mutex
	started bool
	closed  bool

	logger *zap.Logger // non-nil logger.
}

func (s *Server) queryTimeout() time.Duration {
	if t := s.QueryTimeout; t > 0 {
		return t
	}
	return defaultQueryTimeout
}

func (s *Server) idleTimeout() time.Duration {
	if t := s.IdleTimeout; t > 0 {
		return t
	}
	return defaultIdleTimeout
}

// Start starts the udp server.
// Start always returns an non-nil err.
// If server was closed, an ErrServerClosed will be returned.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrServerClosed
	}
	if s.started {
		s.mu.Unlock()
		return ErrServerStarted
	}
	s.started = true
	s.mu.Unlock()

	defer s.Close()

	if s.Handler == nil {
		return errors.New("nil handler")
	}

	if logger := s.Logger; logger != nil {
		s.logger = logger
	} else {
		s.logger = zap.NewNop()
	}

	err := s.startServer()
	if err != nil {
		if s.isClosed() {
			return ErrServerClosed
		}
		return err
	}
	return errServerExited
}

func (s *Server) startServer() error {
	switch s.Protocol {
	case ProtocolUDP:
		return s.startUDP()
	case ProtocolTCP, ProtocolDoT:
		return s.startTCP()
	case ProtocolDoH, ProtocolHttp:
		return s.startDoH()
	default:
		return fmt.Errorf("unknown protocol %d", s.Protocol)
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Listener != nil {
		s.Listener.Close()
	}
	if s.PacketConn != nil {
		s.PacketConn.Close()
	}

	s.closed = true
	return nil
}

func (s *Server) handleQueryTimeout(ctx context.Context, qCtx *handler.Context, w ResponseWriter) {
	queryCtx, cancel := context.WithTimeout(ctx, s.queryTimeout())
	defer cancel()
	s.Handler.ServeDNS(queryCtx, qCtx, w)
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func checkTLSConfig(c *tls.Config) error {
	if c == nil {
		return errors.New("nil tls config")
	}
	if len(c.Certificates) == 0 && c.GetCertificate == nil {
		return errors.New("no certificate")
	}
	return nil
}

func ParseProtocol(s string) (p Protocol, err error) {
	switch s {
	case "", "udp":
		p = ProtocolUDP
	case "tcp":
		p = ProtocolTCP
	case "dot", "tls":
		p = ProtocolDoT
	case "doh", "https":
		p = ProtocolDoT
	case "http":
		p = ProtocolHttp
	default:
		err = fmt.Errorf("unsupported protocol: %s", s)
	}
	return p, err
}
