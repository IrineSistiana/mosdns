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

package server

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
	"time"
)

const PluginType = "server"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

type serverPlugin struct {
	tag    string
	logger *logrus.Entry
	args   *Args

	servers []*Server
}

func (sp *serverPlugin) Tag() string {
	return sp.tag
}

func (sp *serverPlugin) Type() string {
	return PluginType
}

type Args struct {
	Server               []*ServerConfig `yaml:"server"`
	Entry                string          `yaml:"entry"`
	MaxConcurrentQueries int             `yaml:"max_concurrent_queries"`
}

type ServerConfig struct {
	// Protocol: server protocol, can be:
	// "", "udp" -> udp
	// "tcp" -> tcp
	// "dot" -> dns over tls
	// "doh", "https" -> dns over https (rfc 8844)
	// "http" -> dns over https (rfc 8844) but with out tls
	Protocol string `yaml:"protocol"`

	// Addr: server "host:port" addr, "port" can be omitted.
	// Addr can not be empty.
	Addr string `yaml:"addr"`

	Cert    string `yaml:"cert"`     // certificate path, used by dot, doh
	Key     string `yaml:"key"`      // certificate key path, used by dot, doh
	URLPath string `yaml:"url_path"` // used by doh, url path. If it's emtpy, any path will be handled.

	Timeout     uint `yaml:"timeout"`      // (sec) used by all protocol as query timeout, default is defaultQueryTimeout.
	IdleTimeout uint `yaml:"idle_timeout"` // (sec) used by tcp, dot, doh as connection idle timeout, default is defaultIdleTimeout.
}

const (
	defaultQueryTimeout = time.Second * 5
	defaultIdleTimeout  = time.Second * 10
)

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.Server) == 0 {
		return nil, errors.New("no server")
	}
	if len(args.Entry) == 0 {
		return nil, errors.New("empty entry")
	}

	logger := mlog.NewPluginLogger(tag)
	sp := &serverPlugin{
		tag:    tag,
		logger: logger,
		args:   args,
	}

	h := handler.NewDefaultServerHandler(&handler.DefaultServerHandlerConfig{
		Logger:          logger,
		Entry:           args.Entry,
		ConcurrentLimit: args.MaxConcurrentQueries,
	})

	errChan := make(chan error, len(args.Server))
	for _, config := range args.Server {
		s := &Server{
			Logger:  logger,
			Config:  config,
			Handler: h,
		}

		sp.servers = append(sp.servers, s)
		go func() {
			errChan <- s.ListenAndServe()
		}()
	}

	go func() {
		for i := 0; i < len(args.Server); i++ {
			err := <-errChan
			if err != nil {
				sp.shutdownAll()
				handler.PluginFatalErr(tag, fmt.Sprintf("server exited with err: %v", err))
			}
		}
	}()

	return sp, nil
}

func (sp *serverPlugin) shutdownAll() {
	for _, s := range sp.servers {
		s.Shutdown()
	}
}

type Server struct {
	// all of those following members can not be nil
	Logger  *logrus.Entry
	Config  *ServerConfig
	Handler handler.ServerHandler

	l           sync.Mutex // protect the followings
	isActivated bool
	done        bool
	listener    net.Listener
	packetConn  net.PacketConn

	// will only be updated in Server.ListenAndServe() once, can be accessed freely.
	queryTimeout time.Duration
	idleTimeout  time.Duration
}

func (s *Server) ListenAndServe() error {
	s.l.Lock()
	if s.isActivated {
		s.l.Unlock()
		return errors.New("server has been activated")
	}
	s.isActivated = true
	s.l.Unlock()

	if len(s.Config.Addr) == 0 {
		return errors.New("server addr is empty")
	}

	s.queryTimeout = defaultQueryTimeout
	if s.Config.Timeout > 0 {
		s.queryTimeout = time.Duration(s.Config.Timeout) * time.Second
	}

	s.idleTimeout = defaultIdleTimeout
	if s.Config.IdleTimeout > 0 {
		s.idleTimeout = time.Duration(s.Config.IdleTimeout) * time.Second
	}

	// listen
	switch s.Config.Protocol {
	case "", "udp":
		utils.TryAddPort(s.Config.Addr, 53)
		return s.serveUDP()
	case "tcp":
		utils.TryAddPort(s.Config.Addr, 53)
		return s.serveTCP(false)
	case "dot":
		utils.TryAddPort(s.Config.Addr, 853)
		return s.serveTCP(true)
	case "doh", "https":
		utils.TryAddPort(s.Config.Addr, 443)
		return s.serveDoH(false)
	case "http":
		utils.TryAddPort(s.Config.Addr, 80)
		return s.serveDoH(true)
	default:
		return fmt.Errorf("unsupported protocol: %s", s.Config.Protocol)
	}
}

func (s *Server) Shutdown() error {
	s.l.Lock()
	defer s.l.Unlock()
	if !s.isActivated {
		return errors.New("server has not been activated yet")
	}
	s.done = true
	if s.listener != nil {
		s.listener.Close()
	}
	if s.packetConn != nil {
		s.packetConn.Close()
	}
	return nil
}

func (s *Server) isDone() bool {
	s.l.Lock()
	defer s.l.Unlock()
	return s.done
}
