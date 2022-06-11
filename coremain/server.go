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

package coremain

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/server"
	"github.com/IrineSistiana/mosdns/v4/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/v4/pkg/server/http_handler"
	"go.uber.org/zap"
	"net"
	"time"
)

const defaultQueryTimeout = time.Second * 5
const (
	defaultIdleTimeout = time.Second * 10
)

func (m *Mosdns) startServers(cfg *ServerConfig) error {
	if len(cfg.Listeners) == 0 {
		return errors.New("no server listener is configured")
	}
	if len(cfg.Exec) == 0 {
		return errors.New("empty entry")
	}

	entry := m.execs[cfg.Exec]
	if entry == nil {
		return fmt.Errorf("cannot find entry %s", cfg.Exec)
	}

	queryTimeout := defaultQueryTimeout
	if cfg.Timeout > 0 {
		queryTimeout = time.Duration(cfg.Timeout) * time.Second
	}

	dnsHandler := &dns_handler.DefaultHandler{
		Logger:             m.logger,
		Entry:              entry,
		QueryTimeout:       queryTimeout,
		RecursionAvailable: true,
	}

	for _, lc := range cfg.Listeners {
		if err := m.startServerListener(lc, dnsHandler); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mosdns) startServerListener(cfg *ServerListenerConfig, dnsHandler dns_handler.Handler) error {
	if len(cfg.Addr) == 0 {
		return errors.New("no address to bind")
	}

	m.logger.Info("starting server", zap.String("proto", cfg.Protocol), zap.String("addr", cfg.Addr))

	idleTimeout := defaultIdleTimeout
	if cfg.IdleTimeout > 0 {
		idleTimeout = time.Duration(cfg.IdleTimeout) * time.Second
	}
	s := &server.Server{
		DNSHandler: dnsHandler,
		HttpHandler: &http_handler.Handler{
			DNSHandler:  dnsHandler,
			Path:        cfg.URLPath,
			SrcIPHeader: cfg.GetUserIPFromHeader,
			Logger:      m.logger,
		},
		Cert:        cfg.Cert,
		Key:         cfg.Key,
		IdleTimeout: idleTimeout,
		Logger:      m.logger,
	}

	var run func() error
	switch cfg.Protocol {
	case "", "udp":
		conn, err := net.ListenPacket("udp", cfg.Addr)
		if err != nil {
			return err
		}
		run = func() error { return s.ServeUDP(conn) }
	case "tcp":
		l, err := net.Listen("tcp", cfg.Addr)
		if err != nil {
			return err
		}
		run = func() error { return s.ServeTCP(l) }
	case "tls", "dot":
		l, err := net.Listen("tcp", cfg.Addr)
		if err != nil {
			return err
		}
		run = func() error { return s.ServeTLS(l) }
	case "http":
		l, err := net.Listen("tcp", cfg.Addr)
		if err != nil {
			return err
		}
		run = func() error { return s.ServeHTTP(l) }
	case "https", "doh":
		l, err := net.Listen("tcp", cfg.Addr)
		if err != nil {
			return err
		}
		run = func() error { return s.ServeHTTPS(l) }
	default:
		return fmt.Errorf("unknown protocol: [%s]", cfg.Protocol)
	}

	m.sc.Attach(func(done func(), closeSignal <-chan struct{}) {
		defer done()
		errChan := make(chan error, 1)
		go func() {
			errChan <- run()
		}()
		select {
		case err := <-errChan:
			m.sc.SendCloseSignal(fmt.Errorf("server exited, %w", err))
		case <-closeSignal:
		}
	})

	return nil
}
