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
	"errors"
	"net/http"
	"time"
)

func (s *Server) startDoH() error {
	if s.Listener == nil {
		return errors.New("doh server has a nil listener")
	}
	l := s.Listener

	httpServer := &http.Server{
		Handler: &DoHHandler{
			Logger:              s.logger,
			URLPath:             s.URLPath,
			GetUserIPFromHeader: s.GetUserIPFromHeader,
			QueryTimeout:        s.queryTimeout(),
			DNSHandler:          s.Handler,
		},
		TLSConfig:      s.TLSConfig,
		ReadTimeout:    time.Second * 5,
		WriteTimeout:   time.Second * 5,
		IdleTimeout:    s.IdleTimeout,
		MaxHeaderBytes: 2048,
	}

	if s.Protocol == ProtocolHttp {
		return httpServer.Serve(l)
	} else {
		if err := checkTLSConfig(s.TLSConfig); err != nil {
			return err
		}
		return httpServer.ServeTLS(l, "", "")
	}
}
