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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server/http_handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"time"
)

const PluginType = "server"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	Entry                []interface{}   `yaml:"entry"`
	MaxConcurrentQueries int             `yaml:"max_concurrent_queries"`
	Timeout              uint            `yaml:"timeout"` // (sec) query timeout.
	Server               []*ServerConfig `yaml:"server"`
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
	URLPath             string `yaml:"url_path"`                // used by doh, http. If it's empty, any path will be handled.
	GetUserIPFromHeader string `yaml:"get_user_ip_from_header"` // used by doh, http.

	// Deprecated and will not take effect. Use Args.Timeout instead.
	// TODO: Remove deprecated.
	Timeout     uint `yaml:"timeout"`
	IdleTimeout uint `yaml:"idle_timeout"` // (sec) used by tcp, dot, doh as connection idle timeout.
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newServerPlugin(bp, args.(*Args))
}

type serverPlugin struct {
	*handler.BP

	errChan chan error // A buffed chan.

	mu           sync.Mutex
	closed       bool
	serverCloser map[*io.Closer]struct{}
	closeNotify  chan struct{}
}

func newServerPlugin(bp *handler.BP, args *Args) (*serverPlugin, error) {
	if len(args.Server) == 0 {
		return nil, errors.New("no server")
	}
	if len(args.Entry) == 0 {
		return nil, errors.New("empty entry")
	}

	ecs, err := executable_seq.ParseExecutableNode(args.Entry, bp.L())
	if err != nil {
		return nil, err
	}

	dnsHandler := &dns_handler.DefaultHandler{
		Logger:             bp.L(),
		Entry:              ecs,
		QueryTimeout:       time.Duration(args.Timeout) * time.Second,
		ConcurrentLimit:    args.MaxConcurrentQueries,
		RecursionAvailable: true,
	}

	sg := &serverPlugin{
		BP:           bp,
		errChan:      make(chan error, len(args.Server)),
		serverCloser: make(map[*io.Closer]struct{}),
		closeNotify:  make(chan struct{}),
	}

	for _, sc := range args.Server {
		sc := sc
		go func() {
			err := sg.startServer(sc, dnsHandler)
			if err != nil && !sg.isClosed() {
				sg.errChan <- err
			}
		}()
	}

	go func() {
		if err := sg.waitErr(); err != nil {
			handler.PluginFatalErr(bp.Tag(), fmt.Sprintf("server exited with err: %v", err))
		}
	}()

	return sg, nil
}

func (sg *serverPlugin) startServer(c *ServerConfig, dnsHandler dns_handler.Handler) error {
	if len(c.Addr) == 0 {
		return errors.New("no address to bind")
	}

	idleTimeout := time.Duration(c.IdleTimeout) * time.Second
	logger := sg.L()

	s := &server.Server{
		DNSHandler: dnsHandler,
		HttpHandler: &http_handler.Handler{
			DNSHandler:  dnsHandler,
			Path:        c.URLPath,
			SrcIPHeader: c.GetUserIPFromHeader,
			Logger:      logger,
		},
		Cert:        c.Cert,
		Key:         c.Key,
		IdleTimeout: idleTimeout,
		Logger:      logger,
	}

	closer := io.Closer(s)
	var run func() error
	switch c.Protocol {
	case "", "udp":
		conn, err := net.ListenPacket("udp", c.Addr)
		if err != nil {
			return err
		}
		defer conn.Close()
		run = func() error { return s.ServeUDP(conn) }
	case "tcp":
		l, err := net.Listen("tcp", c.Addr)
		if err != nil {
			return err
		}
		defer l.Close()
		run = func() error { return s.ServeTCP(l) }
	case "tls", "dot":
		l, err := net.Listen("tcp", c.Addr)
		if err != nil {
			return err
		}
		defer l.Close()
		run = func() error { return s.ServeTLS(l) }
	case "http":
		l, err := net.Listen("tcp", c.Addr)
		if err != nil {
			return err
		}
		defer l.Close()
		run = func() error { return s.ServeHTTP(l) }
	case "https", "doh":
		l, err := net.Listen("tcp", c.Addr)
		if err != nil {
			return err
		}
		defer l.Close()
		run = func() error { return s.ServeHTTPS(l) }
	default:
		return fmt.Errorf("unknown protocol: [%s]", c.Protocol)
	}

	// TODO: Remove deprecated.
	if c.Timeout > 0 {
		sg.L().Warn("the server timeout argument has been moved to plugin arguments and will not take effect", zap.String("proto", c.Protocol), zap.String("addr", c.Addr))
	}

	sg.L().Info("server started", zap.String("proto", c.Protocol), zap.String("addr", c.Addr))

	if ok := sg.trackCloser(&closer, true); !ok {
		return server.ErrServerClosed
	}
	defer sg.trackCloser(&closer, false)
	return run()
}

func (sg *serverPlugin) Shutdown() error {
	sg.mu.Lock()
	defer sg.mu.Unlock()
	sg.closed = true

	errs := new(utils.Errors)
	for closer := range sg.serverCloser {
		err := (*closer).Close()
		if err != nil {
			errs.Append(err)
		}
	}
	return errs.Build()
}

func (sg *serverPlugin) isClosed() bool {
	sg.mu.Lock()
	defer sg.mu.Unlock()
	return sg.closed
}

func (sg *serverPlugin) trackCloser(c *io.Closer, add bool) bool {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	if add {
		if sg.closed {
			return false
		}
		sg.serverCloser[c] = struct{}{}
	} else {
		delete(sg.serverCloser, c)
	}
	return true
}

func (sg *serverPlugin) waitErr() error {
	select {
	case err := <-sg.errChan:
		return err
	case <-sg.closeNotify:
		return nil
	}
}
