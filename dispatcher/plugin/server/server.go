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
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server_handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"io"
	"sync"
	"time"
)

const PluginType = "server"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type ServerGroup struct {
	*handler.BP
	configs []*Server

	handler server_handler.DNSServerHandler

	m         sync.Mutex
	activated bool
	listener  map[io.Closer]struct{}

	shutdownOnce sync.Once
	shutdownChan chan struct{}
	errChan      chan error
}

type Args struct {
	Server               []*Server     `yaml:"server"`
	Entry                []interface{} `yaml:"entry"`
	MaxConcurrentQueries int           `yaml:"max_concurrent_queries"`
}

// Server is not safe for concurrent use.
type Server struct {
	// Protocol: server protocol, can be:
	// "", "udp" -> udp
	// "tcp" -> tcp
	// "dot", "tls" -> dns over tls
	// "doh", "https" -> dns over https (rfc 8844)
	// "http" -> dns over https (rfc 8844) but without tls
	Protocol string `yaml:"protocol"`

	// Addr: server "host:port" addr, "port" can be omitted.
	// Addr cannot be empty.
	Addr string `yaml:"addr"`

	Cert                string `yaml:"cert"`                    // certificate path, used by dot, doh
	Key                 string `yaml:"key"`                     // certificate key path, used by dot, doh
	URLPath             string `yaml:"url_path"`                // used by doh, http. If it's emtpy, any path will be handled.
	GetUserIPFromHeader string `yaml:"get_user_ip_from_header"` // used by doh, http.

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
	return newServerPlugin(bp, args.(*Args))
}

func newServerPlugin(bp *handler.BP, args *Args) (*ServerGroup, error) {
	if len(args.Server) == 0 {
		return nil, errors.New("no server")
	}
	if len(args.Entry) == 0 {
		return nil, errors.New("empty entry")
	}

	ecs, err := executable_seq.ParseExecutableCmdSequence(args.Entry)
	if err != nil {
		return nil, err
	}

	sh := server_handler.NewDefaultServerHandler(&server_handler.DefaultServerHandlerConfig{
		Logger:          bp.L(),
		Entry:           ecs,
		ConcurrentLimit: args.MaxConcurrentQueries,
	})

	sg := NewServerGroup(bp, sh, args.Server)
	if err := sg.Activate(); err != nil {
		return nil, err
	}
	go func() {
		if err := sg.WaitErr(); err != nil {
			handler.PluginFatalErr(bp.Tag(), fmt.Sprintf("server exited with err: %v", err))
		}
	}()

	return sg, nil
}

func NewServerGroup(bp *handler.BP, handler server_handler.DNSServerHandler, configs []*Server) *ServerGroup {
	s := &ServerGroup{
		BP:      bp,
		configs: configs,
		handler: handler,

		shutdownChan: make(chan struct{}),
		errChan:      make(chan error, len(configs)), // must be a buf chan to avoid blocking.
		listener:     map[io.Closer]struct{}{},
	}
	return s
}

func (sg *ServerGroup) isClosed() bool {
	select {
	case _, alive := <-sg.shutdownChan:
		return !alive
	default:
		return false
	}
}

func (sg *ServerGroup) Shutdown() error {
	sg.m.Lock()
	defer sg.m.Unlock()
	return sg.shutdownNoLock()
}

func (sg *ServerGroup) shutdownNoLock() error {
	sg.shutdownOnce.Do(func() {
		for l := range sg.listener {
			l.Close()
			delete(sg.listener, l)
		}
		close(sg.shutdownChan)
	})
	return nil
}

func (sg *ServerGroup) Activate() error {
	sg.m.Lock()
	defer sg.m.Unlock()

	if sg.activated {
		return errors.New("server has been activated")
	}
	sg.activated = true

	for _, conf := range sg.configs {
		err := sg.listenAndStart(conf)
		if err != nil {
			sg.shutdownNoLock()
			return err
		}
	}
	return nil
}

func (sg *ServerGroup) WaitErr() error {
	select {
	case err := <-sg.errChan:
		return err
	case <-sg.shutdownChan:
		return nil
	}
}

// remainder: listenAndStart should be called only after ServerGroup is locked.
func (sg *ServerGroup) listenAndStart(c *Server) error {
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
		return sg.startUDP(c)
	case "tcp":
		utils.TryAddPort(c.Addr, 53)
		return sg.startTCP(c, false)
	case "dot", "tls":
		utils.TryAddPort(c.Addr, 853)
		return sg.startTCP(c, true)
	case "doh", "https":
		utils.TryAddPort(c.Addr, 443)
		return sg.startDoH(c, false)
	case "http":
		utils.TryAddPort(c.Addr, 80)
		return sg.startDoH(c, true)
	default:
		return fmt.Errorf("unsupported protocol: %s", c.Protocol)
	}
}
