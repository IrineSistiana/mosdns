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
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server/http_handler"
	"sync"
	"time"
)

const PluginType = "server"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	Server               []*ServerConfig `yaml:"server"`
	Entry                []interface{}   `yaml:"entry"`
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

	// Addr: server "host:port" addr.
	// Addr cannot be empty.
	Addr string `yaml:"addr"`

	Cert                string `yaml:"cert"`                    // certificate path, used by dot, doh
	Key                 string `yaml:"key"`                     // certificate key path, used by dot, doh
	URLPath             string `yaml:"url_path"`                // used by doh, http. If it's emtpy, any path will be handled.
	GetUserIPFromHeader string `yaml:"get_user_ip_from_header"` // used by doh, http.

	Timeout     uint `yaml:"timeout"`      // (sec) used by all protocol as query timeout, default is defaultQueryTimeout.
	IdleTimeout uint `yaml:"idle_timeout"` // (sec) used by tcp, dot, doh as connection idle timeout, default is defaultIdleTimeout.
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newServerPlugin(bp, args.(*Args))
}

type serverPlugin struct {
	*handler.BP

	errChan chan error // A buffed chan.

	mu          sync.Mutex
	servers     map[*server.Server]struct{}
	closeNotify chan struct{}
}

func newServerPlugin(bp *handler.BP, args *Args) (*serverPlugin, error) {
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

	sh := &dns_handler.DefaultHandler{
		Logger:          bp.L(),
		Entry:           ecs,
		ConcurrentLimit: args.MaxConcurrentQueries,
	}

	sg := &serverPlugin{
		BP:      bp,
		errChan: make(chan error, len(args.Server)),

		servers:     make(map[*server.Server]struct{}, len(args.Server)),
		closeNotify: make(chan struct{}),
	}

	for _, sc := range args.Server {
		s := server.NewServer(
			sc.Protocol,
			sc.Addr,
			server.WithHandler(sh),
			server.WithHttpHandler(http_handler.NewHandler(
				sh,
				http_handler.WithPath(sc.URLPath),
				http_handler.WithClientSrcIPHeader(sc.GetUserIPFromHeader),
				http_handler.WithTimeout(time.Duration(sc.Timeout)*time.Second),
			)),
			server.WithCertificate(sc.Cert, sc.Key),
			server.WithQueryTimeout(time.Duration(sc.Timeout)*time.Second),
			server.WithIdleTimeout(time.Duration(sc.IdleTimeout)*time.Second),
		)

		sg.mu.Lock()
		sg.servers[s] = struct{}{}
		sg.mu.Unlock()

		go func() {
			err := s.Start()
			if err == server.ErrServerClosed {
				return
			}
			sg.errChan <- err
			sg.mu.Lock()
			delete(sg.servers, s)
			sg.mu.Unlock()
		}()
	}

	go func() {
		if err := sg.waitErr(); err != nil {
			handler.PluginFatalErr(bp.Tag(), fmt.Sprintf("server exited with err: %v", err))
		}
	}()

	return sg, nil
}

func (sg *serverPlugin) Shutdown() error {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	for server := range sg.servers {
		server.Close()
		delete(sg.servers, server)
	}
	return nil
}

func (sg *serverPlugin) waitErr() error {
	select {
	case err := <-sg.errChan:
		return err
	case <-sg.closeNotify:
		return nil
	}
}
