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

package httpserver

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

const PluginType = "http_server"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

type httpServer struct {
	tag        string
	path       string
	logger     *logrus.Entry
	dnsHandler handler.ServerHandler

	server *http.Server
}

func (s *httpServer) Tag() string {
	return s.tag
}

func (s *httpServer) Type() string {
	return PluginType
}

type Args struct {
	Listen               string `yaml:"listen"`
	Path                 string `yaml:"path"`
	Cert                 string `yaml:"cert"`
	Key                  string `yaml:"key"`
	Entry                string `yaml:"entry"`
	MaxConcurrentQueries int    `yaml:"max_concurrent_queries"`
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

	return startNewServer(tag, args)
}

func startNewServer(tag string, args *Args) (*httpServer, error) {
	logger := mlog.NewPluginLogger(tag)
	s := &httpServer{
		tag:    tag,
		path:   args.Path,
		logger: logger,
		dnsHandler: handler.NewDefaultServerHandler(&handler.DefaultServerHandlerConfig{
			Logger:          logger,
			Entry:           args.Entry,
			ConcurrentLimit: args.MaxConcurrentQueries,
		}),
	}

	l, err := net.Listen("tcp", args.Listen)
	if err != nil {
		return nil, err
	}
	s.logger.Infof("http server is running at %s", l.Addr())
	s.server = &http.Server{
		Handler:        s,
		ReadTimeout:    time.Second * 5,
		WriteTimeout:   time.Second * 5,
		IdleTimeout:    time.Minute,
		MaxHeaderBytes: 2048,
	}
	go func() {
		var err error
		switch {
		case len(args.Key) != 0 && len(args.Cert) != 0:
			err = s.server.ServeTLS(l, args.Cert, args.Key)
		default:
			err = s.server.Serve(l)
		}

		if err != nil && err != http.ErrServerClosed {
			handler.PluginFatalErr(tag, err.Error())
		} else {
			s.logger.Infof("http server %s is closed", l.Addr())
		}
	}()
	return s, nil
}

func (s *httpServer) close() error {
	return s.server.Close()
}

func (s *httpServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if len(s.path) != 0 && req.URL.Path != s.path {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	q, err := s.getMsg(req)
	if err != nil {
		s.logger.Warnf("invalid request from %s: %v", req.RemoteAddr, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	qCtx := handler.NewContext(q)
	qCtx.From = httpAddr(req.RemoteAddr)

	responseWriter := &httpDnsRespWriter{httpRespWriter: w}
	s.dnsHandler.ServeDNS(req.Context(), qCtx, responseWriter)
}

func (s *httpServer) getMsg(req *http.Request) (*dns.Msg, error) {
	var b []byte
	var err error
	switch req.Method {
	case http.MethodGet:
		s := req.URL.Query().Get("dns")
		if len(s) == 0 {
			return nil, fmt.Errorf("no dns parameter in url %s", req.RequestURI)
		}
		b, err = base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("failed to decode url %s: %w", req.RequestURI, err)
		}
	case http.MethodPost:
		b, err = ioutil.ReadAll(io.LimitReader(req.Body, dns.MaxMsgSize))
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}

	q := new(dns.Msg)
	if err := q.Unpack(b); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	return q, nil
}

type httpAddr string

func (h httpAddr) Network() string {
	return "http"
}

func (h httpAddr) String() string {
	return string(h)
}

type httpDnsRespWriter struct {
	httpRespWriter http.ResponseWriter
}

func (h *httpDnsRespWriter) Write(m *dns.Msg) (n int, err error) {
	return utils.WriteMsgToUDP(h.httpRespWriter, m)
}
