//     Copyright (C) 2020, IrineSistiana
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

package plainserver

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/logger"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"io"
	"net"
)

const PluginType = "plain_server"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

type plainServer struct {
	tag  string
	args *Args

	listener []io.Closer
}

func (s *plainServer) Tag() string {
	return s.tag
}

func (s *plainServer) Type() string {
	return PluginType
}

type Args struct {
	Listen []string `yaml:"listen"`
	Entry  string   `yaml:"entry"`
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.Listen) == 0 {
		return nil, errors.New("no address to listen")
	}

	s := &plainServer{
		tag:  tag,
		args: args,
	}

	h := &handler.DefaultServerHandler{Entry: s.args.Entry}
	for _, addr := range args.Listen {
		protocol, host := utils.ParseAddr(addr)
		addr = utils.TryAddPort(host, 53)
		switch protocol {
		case "", "udp":
			l, err := net.ListenPacket("udp", addr)
			if err != nil {
				s.shutDown()
				return nil, err
			}
			s.listener = append(s.listener, l)
			logger.Entry().Infof("udp server started at %s", l.LocalAddr())
			go func() {
				err := listenAndServeUDP(l, h)
				logger.Entry().Fatalf("udp server at %s exited: %v", l.LocalAddr(), err)
			}()
		case "tcp":
			l, err := net.Listen("tcp", addr)
			if err != nil {
				s.shutDown()
				return nil, err
			}
			s.listener = append(s.listener, l)
			logger.Entry().Infof("tcp server started at %s", l.Addr())
			go func() {
				err := listenAndServeTCP(l, h)
				logger.Entry().Fatalf("tcp server at %s exited: %v", l.Addr(), err)
			}()
		default:
			return nil, fmt.Errorf("unsupported protocol: %s", protocol)
		}
	}
	return s, nil
}

// shutDown closes all server listeners.
func (s *plainServer) shutDown() {
	for _, l := range s.listener {
		l.Close()
	}
}
