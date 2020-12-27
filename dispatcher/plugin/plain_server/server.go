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
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/sirupsen/logrus"
	"io"
	"net"
)

const PluginType = "plain_server"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

type plainServerPlugin struct {
	tag          string
	logger       *logrus.Entry
	args         *Args
	shutdownChan chan struct{}

	startedListener []io.Closer
}

func (s *plainServerPlugin) Tag() string {
	return s.tag
}

func (s *plainServerPlugin) Type() string {
	return PluginType
}

type Args struct {
	Listen               []string `yaml:"listen"`
	Entry                string   `yaml:"entry"`
	MaxConcurrentQueries int      `yaml:"max_concurrent_queries"`
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
	if len(args.Entry) == 0 {
		return nil, errors.New("empty entry")
	}

	logger := mlog.NewPluginLogger(tag)
	sp := &plainServerPlugin{
		tag:          tag,
		logger:       logger,
		args:         args,
		shutdownChan: make(chan struct{}),
	}

	h := handler.NewDefaultServerHandler(&handler.DefaultServerHandlerConfig{
		Logger:          logger,
		Entry:           args.Entry,
		ConcurrentLimit: args.MaxConcurrentQueries,
	})
	server := &singleServer{
		logger:       sp.logger,
		shutdownChan: sp.shutdownChan,
	}
	for _, addr := range args.Listen {
		protocol, host := utils.ParseAddr(addr)
		addr = utils.TryAddPort(host, 53)
		switch protocol {
		case "", "udp":
			l, err := net.ListenPacket("udp", addr)
			if err != nil {
				sp.shutDown()
				return nil, err
			}
			sp.startedListener = append(sp.startedListener, l)
			sp.logger.Infof("udp server started at %s", l.LocalAddr())
			go func() {
				if err := server.serveUDP(l, h); err != nil {
					handler.PluginFatalErr(tag, fmt.Sprintf("udp server %s exited: %v", l.LocalAddr(), err))
				} else {
					sp.logger.Infof("udp server %s exited", l.LocalAddr())
				}
			}()
		case "tcp":
			l, err := net.Listen("tcp", addr)
			if err != nil {
				sp.shutDown()
				return nil, err
			}
			sp.startedListener = append(sp.startedListener, l)
			sp.logger.Infof("tcp server started at %s", l.Addr())
			go func() {
				if err := server.serveTCP(l, h); err != nil {
					handler.PluginFatalErr(tag, fmt.Sprintf("tcp server %s exited: %v", l.Addr(), err))
				} else {
					sp.logger.Infof("tcp server %s exited", l.Addr())
				}
			}()
		default:
			return nil, fmt.Errorf("unsupported protocol: %s", protocol)
		}
	}
	return sp, nil
}

// shutDown closes all server listeners.
func (s *plainServerPlugin) shutDown() {
	close(s.shutdownChan)
	for _, l := range s.startedListener {
		l.Close()
	}
}

type singleServer struct {
	logger       *logrus.Entry
	shutdownChan chan struct{}
}
