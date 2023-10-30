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

package udp_server

import (
	"context"
	"fmt"
	"net"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/server"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/server/server_utils"
	"go.uber.org/zap"
)

const PluginType = "udp_server"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

type Args struct {
	Entry  string `yaml:"entry"`
	Listen string `yaml:"listen"`
}

func (a *Args) init() {
	utils.SetDefaultString(&a.Listen, "127.0.0.1:53")
}

type UdpServer struct {
	args *Args

	c net.PacketConn
}

func (s *UdpServer) Close() error {
	return s.c.Close()
}

func Init(bp *coremain.BP, args any) (any, error) {
	return StartServer(bp, args.(*Args))
}

func StartServer(bp *coremain.BP, args *Args) (*UdpServer, error) {
	dh, err := server_utils.NewHandler(bp, args.Entry)
	if err != nil {
		return nil, fmt.Errorf("failed to init dns handler, %w", err)
	}

	socketOpt := server_utils.ListenerSocketOpts{
		SO_REUSEPORT: true,
		SO_RCVBUF:    64 * 1024,
	}
	lc := net.ListenConfig{Control: server_utils.ListenerControl(socketOpt)}
	c, err := lc.ListenPacket(context.Background(), "udp", args.Listen)
	if err != nil {
		return nil, fmt.Errorf("failed to create socket, %w", err)
	}
	bp.L().Info("udp server started", zap.Stringer("addr", c.LocalAddr()))

	go func() {
		defer c.Close()
		err := server.ServeUDP(c.(*net.UDPConn), dh, server.UDPServerOpts{Logger: bp.L()})
		bp.M().GetSafeClose().SendCloseSignal(err)
	}()
	return &UdpServer{
		args: args,
		c:    c,
	}, nil
}
