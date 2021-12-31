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
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"time"
)

func (s *Server) newHTTPServer() *http.Server {
	return &http.Server{
		Handler:        s.HttpHandler,
		ReadTimeout:    time.Second * 5,
		WriteTimeout:   time.Second * 5,
		IdleTimeout:    s.getIdleTimeout(),
		MaxHeaderBytes: 2048,
	}
}

func (s *Server) ServeHTTP(l net.Listener) error {
	ol := &onceCloseListener{Listener: l}
	defer ol.Close()

	hs := s.newHTTPServer()
	closer := io.Closer(hs)
	if ok := s.trackCloser(&closer, true); !ok {
		return ErrServerClosed
	}
	defer s.trackCloser(&closer, false)

	err := hs.Serve(ol)
	if err == http.ErrServerClosed { // Replace http.ErrServerClosed with our ErrServerClosed
		return ErrServerClosed
	}
	return err
}

func (s *Server) ServeHTTPS(l net.Listener) error {
	ol := &onceCloseListener{Listener: l}
	defer ol.Close()

	hs := s.newHTTPServer()
	hs.TLSConfig = s.TLSConfig.Clone()
	if err := http2.ConfigureServer(hs, &http2.Server{IdleTimeout: s.getIdleTimeout()}); err != nil {
		s.getLogger().Warn("failed to set up http2 support", zap.Error(err))
	}

	closer := io.Closer(hs)
	if ok := s.trackCloser(&closer, true); !ok {
		return ErrServerClosed
	}
	defer s.trackCloser(&closer, false)

	err := hs.ServeTLS(ol, s.Cert, s.Key)
	if err == http.ErrServerClosed { // Replace http.ErrServerClosed with our ErrServerClosed
		return ErrServerClosed
	}
	return err
}
