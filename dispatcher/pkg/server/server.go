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
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server/dns_handler"
	"go.uber.org/zap"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server errors
var (
	ErrServerClosed  = errors.New("server closed")
	ErrServerStarted = errors.New("server has been started")
)

type Server struct {
	protocol    string
	handler     dns_handler.Handler // UDP, TCP, DoT
	httpHandler http.Handler        // DoH, HTTP

	tlsConfig *tls.Config
	key, cert string

	queryTimeout time.Duration // default is defaultQueryTimeout.
	idleTimeout  time.Duration // default is defaultIdleTimeout.

	logger *zap.Logger

	addr       string
	listener   net.Listener
	packetConn net.PacketConn

	mu      sync.Mutex
	started bool
	closed  bool
}

func NewServer(protocol, addr string, options ...ServerOption) *Server {
	s := new(Server)
	s.protocol = protocol
	s.addr = addr
	for _, op := range options {
		op(s)
	}

	if s.logger == nil {
		s.logger = zap.NewNop()
	}
	return s
}

func (s *Server) getQueryTimeout() time.Duration {
	if t := s.queryTimeout; t > 0 {
		return t
	}
	return defaultQueryTimeout
}

func (s *Server) getIdleTimeout() time.Duration {
	if t := s.idleTimeout; t > 0 {
		return t
	}
	return defaultIdleTimeout
}

// Start starts the udp server.
// Start always returns an non-nil err.
// After Close(), an ErrServerClosed will be returned.
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

	var serverErr error
	switch s.protocol {
	case "udp":
		if err := s.initPacketConn(); err != nil {
			return err
		}
		serverErr = s.startUDP()
	case "tcp":
		if err := s.initListener(); err != nil {
			return err
		}
		serverErr = s.startTCP()
	case "dot", "tls":
		if err := s.initListener(); err != nil {
			return err
		}
		serverErr = s.startDoT()
	case "doh", "https":
		if err := s.initListener(); err != nil {
			return err
		}
		serverErr = s.startDoH()
	case "http":
		if err := s.initListener(); err != nil {
			return err
		}
		serverErr = s.startHttp()
	default:
		return fmt.Errorf("unknown protocol %s", s.protocol)
	}

	if s.isClosed() {
		return ErrServerClosed
	}
	return serverErr
}

func (s *Server) initListener() error {
	if s.listener != nil {
		return nil
	}

	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = l
	return nil
}

func (s *Server) initPacketConn() error {
	if s.packetConn != nil {
		return nil
	}

	c, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		return err
	}
	s.packetConn = c
	return nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true

	if s.listener != nil {
		s.listener.Close()
	}
	if s.packetConn != nil {
		s.packetConn.Close()
	}
	return nil
}

func (s *Server) handleQuery(ctx context.Context, qCtx *handler.Context, w dns_handler.ResponseWriter) {
	queryCtx, cancel := context.WithTimeout(ctx, s.getQueryTimeout())
	defer cancel()
	s.handler.ServeDNS(queryCtx, qCtx, w)
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
