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
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/server"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/server/server_utils"
	"go.uber.org/zap"
)

const PluginType = "tcp_server"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

type Args struct {
	Entry       string `yaml:"entry"`
	Listen      string `yaml:"listen"`
	Cert        string `yaml:"cert"`
	Key         string `yaml:"key"`
	IdleTimeout int    `yaml:"idle_timeout"`
}

func (a *Args) init() {
	utils.SetDefaultString(&a.Listen, "127.0.0.1:53")
	utils.SetDefaultNum(&a.IdleTimeout, 10)
}

type TcpServer struct {
	args *Args

	l net.Listener
}

func (s *TcpServer) Close() error {
	return s.l.Close()
}

func Init(bp *coremain.BP, args any) (any, error) {
	return StartServer(bp, args.(*Args))
}

func StartServer(bp *coremain.BP, args *Args) (*TcpServer, error) {
	dh, err := server_utils.NewHandler(bp, args.Entry)
	if err != nil {
		return nil, fmt.Errorf("failed to init dns handler, %w", err)
	}

	// Init tls
	var tc *tls.Config
	if len(args.Key)+len(args.Cert) > 0 {
		tc = new(tls.Config)
		if err := server.LoadCert(tc, args.Cert, args.Key); err != nil {
			return nil, fmt.Errorf("failed to read tls cert, %w", err)
		}
	}

	socketOpt := server_utils.ListenerSocketOpts{
		SO_REUSEPORT: true,
		SO_RCVBUF:    64 * 1024,
	}
	lc := net.ListenConfig{Control: server_utils.ListenerControl(socketOpt)}
	listenerNetwork := "tcp"
	if strings.HasPrefix(args.Listen, "@") {
		listenerNetwork = "unix"
	}
	l, err := lc.Listen(context.Background(), listenerNetwork, args.Listen)
	if err != nil {
		return nil, fmt.Errorf("failed to listen socket, %w", err)
	}
	if tc != nil {
		l = tls.NewListener(l, tc)
	}
	bp.L().Info("tcp server started", zap.Stringer("addr", l.Addr()), zap.Bool("tls", tc != nil))

	go func() {
		defer l.Close()
		serverOpts := server.TCPServerOpts{Logger: bp.L(), IdleTimeout: time.Duration(args.IdleTimeout) * time.Second}
		err := server.ServeTCP(l, dh, serverOpts)
		bp.M().GetSafeClose().SendCloseSignal(err)
	}()
	return &TcpServer{
		args: args,
		l:    l,
	}, nil
}
