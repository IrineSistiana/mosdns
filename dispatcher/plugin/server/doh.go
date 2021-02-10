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
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server_handler"
	"go.uber.org/zap"
	"net"
	"net/http"
	"time"
)

// remainder: startDoH should be called only after ServerGroup is locked.
func (sg *ServerGroup) startDoH(conf *Server, noTLS bool) error {
	if !noTLS && (len(conf.Cert) == 0 || len(conf.Key) == 0) { // no cert
		return errors.New("doh server needs cert and key")
	}

	l, err := net.Listen("tcp", conf.Addr)
	if err != nil {
		return err
	}

	sg.listener[l] = struct{}{}

	httpServer := &http.Server{
		Handler: &server_handler.DoHHandler{
			Logger:              sg.L(),
			URLPath:             conf.URLPath,
			GetUserIPFromHeader: conf.GetUserIPFromHeader,
			QueryTimeout:        conf.queryTimeout,
			DNSHandler:          sg.handler,
		},
		ReadTimeout:    time.Second * 5,
		WriteTimeout:   time.Second * 5,
		IdleTimeout:    conf.idleTimeout,
		MaxHeaderBytes: 2048,
	}

	go func() {
		sg.L().Info("doh server started", zap.Stringer("addr", l.Addr()))
		defer sg.L().Info("doh server exited", zap.Stringer("addr", l.Addr()))

		var err error
		if noTLS {
			err = httpServer.Serve(l)
		} else {
			err = httpServer.ServeTLS(l, conf.Cert, conf.Key)
		}
		if err != nil {
			sg.errChan <- err
		}
	}()

	return nil
}
