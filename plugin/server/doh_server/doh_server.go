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

package tcp_server

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/server/http_handler"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/server/server_utils"
	"net/http"
	"time"
)

const PluginType = "doh_server"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	Entries []struct {
		Exec string `yaml:"exec"`
		Path string `yaml:"path"`
	} `yaml:"entries"`
	Listen      string `yaml:"listen"`
	SrcIPHeader string `yaml:"src_ip_header"`
	Cert        string `yaml:"cert"`
	Key         string `yaml:"key"`
	IdleTimeout int    `yaml:"idleTimeout"`
}

func (a *Args) init() {
	utils.SetDefaultNum(&a.IdleTimeout, 30)
}

type DoHServer struct {
	*coremain.BP
	args *Args

	server *http.Server
}

func (s *DoHServer) Close() error {
	return s.server.Close()
}

func Init(bp *coremain.BP, args interface{}) (coremain.Plugin, error) {
	return StartServer(bp, args.(*Args))
}

func StartServer(bp *coremain.BP, args *Args) (*DoHServer, error) {
	mux := http.NewServeMux()
	for _, entry := range args.Entries {
		dh, err := server_utils.NewHandler(bp, entry.Exec)
		if err != nil {
			return nil, fmt.Errorf("failed to init dns handler, %w", err)
		}
		hhOpts := http_handler.HandlerOpts{
			DNSHandler:  dh,
			SrcIPHeader: args.SrcIPHeader,
			Logger:      bp.L(),
		}
		hh := http_handler.NewHandler(hhOpts)
		mux.Handle(entry.Path, hh)
	}

	hs := &http.Server{
		Addr:           args.Listen,
		Handler:        mux,
		ReadTimeout:    time.Second,
		IdleTimeout:    time.Duration(args.IdleTimeout) * time.Second,
		MaxHeaderBytes: 512,
	}

	go func() {
		var err error
		if len(args.Key)+len(args.Cert) > 0 {
			err = hs.ListenAndServeTLS(args.Cert, args.Key)
		} else {
			err = hs.ListenAndServe()
		}
		bp.M().GetSafeClose().SendCloseSignal(err)
	}()
	return &DoHServer{
		BP:     bp,
		args:   args,
		server: hs,
	}, nil
}
