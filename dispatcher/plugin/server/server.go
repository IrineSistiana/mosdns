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
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server"
	"net"
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

	sh := &server.DefaultServerHandler{
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
		s := new(server.Server)
		s.Handler = sh
		s.QueryTimeout = time.Duration(sc.Timeout) * time.Second
		s.IdleTimeout = time.Duration(sc.IdleTimeout) * time.Second

		var err error
		s.Protocol, err = server.ParseProtocol(sc.Protocol)
		if err != nil {
			return nil, err
		}

		if len(sc.Addr) == 0 {
			return nil, errors.New("empty server address")
		}

		if s.Protocol == server.ProtocolUDP {
			c, err := net.ListenPacket("udp", sc.Addr)
			if err != nil {
				return nil, err
			}
			s.PacketConn = c
		} else {
			c, err := net.Listen("tcp", sc.Addr)
			if err != nil {
				return nil, err
			}
			s.Listener = c
		}

		switch s.Protocol {
		case server.ProtocolDoT, server.ProtocolDoH:
			if len(sc.Key) == 0 || len(sc.Cert) == 0 {
				return nil, errors.New("no certificate")
			}
			cert, err := tls.LoadX509KeyPair(sc.Cert, sc.Key)
			if err != nil {
				return nil, fmt.Errorf("failed to load certificate: %w", err)
			}
			tlsConfig := new(tls.Config)
			tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
			s.TLSConfig = tlsConfig
		}

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

//
func (sg *serverPlugin) waitErr() error {
	select {
	case err := <-sg.errChan:
		return err
	case <-sg.closeNotify:
		return nil
	}
}
