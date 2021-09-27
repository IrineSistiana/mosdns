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
	"crypto/tls"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/server/dns_handler"
	"go.uber.org/zap"
	"net"
	"net/http"
	"time"
)

const (
	defaultQueryTimeout = time.Second * 5
	defaultIdleTimeout  = time.Second * 10
)

type ServerOption func(s *Server)

// WithHandler sets the dns handler for UDP, TCP, DoT server.
// It cannot be omitted.
func WithHandler(h dns_handler.Handler) ServerOption {
	return func(s *Server) {
		s.handler = h
	}
}

// WithHttpHandler sets the http handler for DoH, HTTP server.
// It cannot be omitted.
func WithHttpHandler(h http.Handler) ServerOption {
	return func(s *Server) {
		s.httpHandler = h
	}
}

// WithTLSConfig sets the tls config for DoT, DoH server.
// WithTLSConfig and WithCertificate cannot be omitted at the same time.
func WithTLSConfig(c *tls.Config) ServerOption {
	return func(s *Server) {
		s.tlsConfig = c
	}
}

// WithCertificate sets the certificate for DoT, DoH server.
// WithTLSConfig and WithCertificate cannot be omitted at the same time.
func WithCertificate(cert, key string) ServerOption {
	return func(s *Server) {
		s.cert = cert
		s.key = key
	}
}

// WithQueryTimeout sets the query maximum executing time.
// Default is defaultQueryTimeout.
func WithQueryTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.queryTimeout = d
	}
}

// WithIdleTimeout sets tcp, dot and doh connections idle timeout.
// Default is defaultIdleTimeout.
func WithIdleTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.idleTimeout = d
	}
}

func WithLogger(l *zap.Logger) ServerOption {
	return func(s *Server) {
		s.logger = l
	}
}

// WithListener sets the TCP listener for TCP, DoT, DoH server.
// With WithListener, addr in NewServer can be empty.
func WithListener(l net.Listener) ServerOption {
	return func(s *Server) {
		s.listener = l
	}
}

// WithPacketConn sets the UDP socket UDP server.
// With WithPacketConn, addr in NewServer can be empty.
func WithPacketConn(c net.PacketConn) ServerOption {
	return func(s *Server) {
		s.packetConn = c
	}
}
