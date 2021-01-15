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
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"io"
	"sync"
	"time"
)

const PluginType = "server"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type server struct {
	*handler.BP
	args *Args

	handler utils.ServerHandler

	m         sync.Mutex
	activated bool
	closed    bool
	closeChan chan struct{}
	errChan   chan error
	listener  map[io.Closer]struct{}
}

type Args struct {
	Server               []*ServerConfig `yaml:"server"`
	Entry                string          `yaml:"entry"`
	MaxConcurrentQueries int             `yaml:"max_concurrent_queries"`
}

// ServerConfig is not safe for concurrent use.
type ServerConfig struct {
	// Protocol: server protocol, can be:
	// "", "udp" -> udp
	// "tcp" -> tcp
	// "dot", "tls" -> dns over tls
	// "doh", "https" -> dns over https (rfc 8844)
	// "http" -> dns over https (rfc 8844) but without tls
	Protocol string `yaml:"protocol"`

	// Addr: server "host:port" addr, "port" can be omitted.
	// Addr can not be empty.
	Addr string `yaml:"addr"`

	Cert    string `yaml:"cert"`     // certificate path, used by dot, doh
	Key     string `yaml:"key"`      // certificate key path, used by dot, doh
	URLPath string `yaml:"url_path"` // used by doh, url path. If it's emtpy, any path will be handled.

	Timeout     uint `yaml:"timeout"`      // (sec) used by all protocol as query timeout, default is defaultQueryTimeout.
	IdleTimeout uint `yaml:"idle_timeout"` // (sec) used by tcp, dot, doh as connection idle timeout, default is defaultIdleTimeout.

	queryTimeout time.Duration
	idleTimeout  time.Duration
}

const (
	defaultQueryTimeout = time.Second * 5
	defaultIdleTimeout  = time.Second * 10
)

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newServer(bp, args.(*Args))
}

func newServer(bp *handler.BP, args *Args) (*server, error) {
	if len(args.Server) == 0 {
		return nil, errors.New("no server")
	}
	if len(args.Entry) == 0 {
		return nil, errors.New("empty entry")
	}

	s := &server{
		BP:   bp,
		args: args,

		handler: utils.NewDefaultServerHandler(&utils.DefaultServerHandlerConfig{
			Logger:          bp.L(),
			Entry:           args.Entry,
			ConcurrentLimit: args.MaxConcurrentQueries,
		}),

		closeChan: make(chan struct{}),
		errChan:   make(chan error, len(args.Server)), // must be a buf chan to avoid block.
		listener:  map[io.Closer]struct{}{},
	}

	err := s.Activate()
	if err != nil {
		return nil, err
	}

	go func() {
		for i := 0; i < len(args.Server); i++ {
			err := <-s.errChan
			if err != nil {
				s.Shutdown()
				handler.PluginFatalErr(bp.Tag(), fmt.Sprintf("server exited with err: %v", err))
			}
		}
	}()
	return s, nil
}

func (s *server) isClosed() bool {
	s.m.Lock()
	defer s.m.Unlock()
	return s.closed
}

func (s *server) Shutdown() error {
	s.m.Lock()
	defer s.m.Unlock()

	return s.shutdownNoLock()
}

func (s *server) shutdownNoLock() error {
	if !s.closed {
		close(s.closeChan) // close chan once
		s.closed = true
	}
	for l := range s.listener {
		err := l.Close()
		if err != nil {
			return err
		} else {
			delete(s.listener, l)
		}
	}
	return nil
}

func (s *server) Activate() error {
	s.m.Lock()
	defer s.m.Unlock()

	if s.activated {
		return errors.New("server has been activated")
	}
	s.activated = true

	for _, conf := range s.args.Server {
		err := s.listenAndStart(conf)
		if err != nil {
			s.shutdownNoLock()
			return err
		}
	}
	return nil
}

func (s *server) listenAndStart(c *ServerConfig) error {
	if len(c.Addr) == 0 {
		return errors.New("server addr is empty")
	}

	c.queryTimeout = defaultQueryTimeout
	if c.Timeout > 0 {
		c.queryTimeout = time.Duration(c.Timeout) * time.Second
	}

	c.idleTimeout = defaultIdleTimeout
	if c.IdleTimeout > 0 {
		c.idleTimeout = time.Duration(c.IdleTimeout) * time.Second
	}

	// start server
	switch c.Protocol {
	case "", "udp":
		utils.TryAddPort(c.Addr, 53)
		return s.startUDP(c)
	case "tcp":
		utils.TryAddPort(c.Addr, 53)
		return s.startTCP(c, false)
	case "dot", "tls":
		utils.TryAddPort(c.Addr, 853)
		return s.startTCP(c, true)
	case "doh", "https":
		utils.TryAddPort(c.Addr, 443)
		return s.startDoH(c, false)
	case "http":
		utils.TryAddPort(c.Addr, 80)
		return s.startDoH(c, true)
	default:
		return fmt.Errorf("unsupported protocol: %s", c.Protocol)
	}
}
