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
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"time"
)

func (s *Server) ServeHTTP(l net.Listener) error {
	return s.serveHTTP(l, false)
}

func (s *Server) ServeHTTPS(l net.Listener) error {
	return s.serveHTTP(l, true)
}

func (s *Server) serveHTTP(l net.Listener, https bool) error {
	defer l.Close()

	if s.opts.HttpHandler == nil {
		return errMissingHTTPHandler
	}

	hs := &http.Server{
		Handler:           s.opts.HttpHandler,
		ReadHeaderTimeout: time.Millisecond * 500,
		ReadTimeout:       time.Second * 5,
		WriteTimeout:      time.Second * 5,
		IdleTimeout:       s.opts.IdleTimeout,
		MaxHeaderBytes:    2048,
		TLSConfig:         s.opts.TLSConfig.Clone(),
	}
	closer := io.Closer(hs)
	if ok := s.trackCloser(&closer, true); !ok {
		return ErrServerClosed
	}
	defer s.trackCloser(&closer, false)

	if err := http2.ConfigureServer(hs, &http2.Server{IdleTimeout: s.opts.IdleTimeout}); err != nil {
		s.opts.Logger.Error("failed to set up http2 support", zap.Error(err))
	}

	var err error
	if https {
		err = hs.ServeTLS(l, s.opts.Cert, s.opts.Key)
	} else {
		err = hs.Serve(l)
	}
	if err == http.ErrServerClosed { // Replace http.ErrServerClosed with our ErrServerClosed
		return ErrServerClosed
	}
	return err
}
