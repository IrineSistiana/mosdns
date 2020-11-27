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

package dispatcher

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/config"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin"
	"github.com/IrineSistiana/mosdns/dispatcher/server"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/IrineSistiana/mosdns/dispatcher/logger"

	"github.com/miekg/dns"
)

const (
	queryTimeout = time.Second * 5
)

// Dispatcher represents a dns query dispatcher
type Dispatcher struct {
	config *config.Config
}

// Init inits a dispatcher from configuration
func Init(c *config.Config) (*Dispatcher, error) {
	// init logger
	if len(c.Log.Level) != 0 {
		level, err := logrus.ParseLevel(c.Log.Level)
		if err != nil {
			return nil, err
		}
		logger.GetStd().SetLevel(level)
	}
	if len(c.Log.File) != 0 {
		f, err := os.OpenFile(c.Log.File, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			return nil, fmt.Errorf("can not open log file %s: %w", c.Log.File, err)
		}
		logger.GetStd().Infof("use log file %s", c.Log.File)
		logWriter := io.MultiWriter(os.Stdout, f)
		logger.GetStd().SetOutput(logWriter)
	}
	if logger.GetStd().IsLevelEnabled(logrus.DebugLevel) {
		logger.GetStd().SetReportCaller(true)
		go func() {
			m := new(runtime.MemStats)
			for {
				time.Sleep(time.Second * 15)
				runtime.ReadMemStats(m)
				logger.GetStd().Debugf("HeapObjects: %d NumGC: %d PauseTotalNs: %d, NumGoroutine: %d", m.HeapObjects, m.NumGC, m.PauseTotalNs, runtime.NumGoroutine())
			}
		}()
	}

	d := new(Dispatcher)
	d.config = c

	for i, pluginConfig := range c.Plugin {
		if len(pluginConfig.Tag) == 0 {
			logger.GetStd().Warnf("plugin at index %d has a empty tag, ignore it", i)
			continue
		}
		if err := handler.RegPlugin(pluginConfig); err != nil {
			return nil, fmt.Errorf("failed to register plugin %d-%s: %w", i, pluginConfig.Tag, err)
		}
	}
	return d, nil
}

func (d *Dispatcher) ServeDNS(ctx context.Context, qCtx *handler.Context, w server.ResponseWriter) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	err := d.Dispatch(queryCtx, qCtx)

	var r *dns.Msg
	if err != nil {
		logger.GetStd().Warnf("query failed: %v", err)
		r = new(dns.Msg)
		r.SetReply(qCtx.Q)
		r.Rcode = dns.RcodeServerFailure
	} else {
		r = qCtx.R
	}

	if r != nil {
		if _, err := w.Write(r); err != nil {
			logger.GetStd().Warnf("failed to response client: %v", err)
		}
	}
}

// Dispatch sends q to entries and return first valid result.
func (d *Dispatcher) Dispatch(ctx context.Context, qCtx *handler.Context) error {
	if len(d.config.Entry) == 0 {
		panic("dispatcher: empty entry")
	}

	if len(d.config.Entry) == 1 {
		return d.dispatchSingleEntry(ctx, qCtx)
	}
	return d.dispatchMultiEntries(ctx, qCtx)
}

func (d *Dispatcher) dispatchMultiEntries(ctx context.Context, qCtx *handler.Context) error {
	resChan := make(chan *dns.Msg, 1)
	upstreamWG := sync.WaitGroup{}
	for i := range d.config.Entry {
		entryTag := d.config.Entry[i]

		upstreamWG.Add(1)
		go func() {
			defer upstreamWG.Done()

			entryQCtx := qCtx.Copy() // qCtx cannot be modified in different goroutine. Copy it.

			queryStart := time.Now()
			err := handler.Walk(ctx, entryQCtx, entryTag)
			rtt := time.Since(queryStart).Milliseconds()
			if err != nil {
				if err != context.Canceled {
					logger.GetStd().Warnf("%v: entry %s returned an err after %dms: %v", qCtx, entryTag, rtt, err)
				}
				return
			}

			if entryQCtx.R != nil {
				logger.GetStd().Debugf("%v: reply from entry %s accepted, rtt: %dms", qCtx, entryTag, rtt)
				select {
				case resChan <- entryQCtx.R:
				default:
				}
			}
		}()
	}

	entriesFailedNotificationChan := make(chan struct{}, 0)
	// this go routine notifies the Dispatch if all entries are failed
	go func() {
		// all entries are returned
		upstreamWG.Wait()
		// avoid below select{} choose entriesFailedNotificationChan
		// if both resChan and entriesFailedNotificationChan are selectable
		if len(resChan) == 0 {
			close(entriesFailedNotificationChan)
		}
	}()

	select {
	case m := <-resChan:
		qCtx.R = m
		return nil
	case <-entriesFailedNotificationChan:
		return errors.New("all entries failed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Dispatcher) dispatchSingleEntry(ctx context.Context, qCtx *handler.Context) error {
	entry := d.config.Entry[0]
	queryStart := time.Now()
	err := handler.Walk(ctx, qCtx, entry)
	rtt := time.Since(queryStart).Milliseconds()
	logger.GetStd().Debugf("%v: entry %s returned after %dms:", qCtx, entry, rtt)
	return err
}

// StartServer starts mosdns. Will always return a non-nil err.
func (d *Dispatcher) StartServer() error {

	if len(d.config.Server.Bind) == 0 {
		return fmt.Errorf("no address to bind")
	}

	errChan := make(chan error, 1) // must be a buffered chan to catch at least one err.

	for _, s := range d.config.Server.Bind {
		ss := strings.Split(s, "://")
		if len(ss) != 2 {
			return fmt.Errorf("invalid bind address: %s", s)
		}
		network := ss[0]
		addr := ss[1]

		var s server.Server
		switch network {
		case "tcp", "tcp4", "tcp6":
			l, err := net.Listen(network, addr)
			if err != nil {
				return err
			}
			defer l.Close()
			logger.GetStd().Infof("tcp server started at %s", l.Addr())

			serverConf := server.Config{
				Listener: l,
			}
			s = server.NewTCPServer(&serverConf)

		case "udp", "udp4", "udp6":
			l, err := net.ListenPacket(network, addr)
			if err != nil {
				return err
			}
			defer l.Close()
			logger.GetStd().Infof("udp server started at %s", l.LocalAddr())
			serverConf := server.Config{
				PacketConn:        l,
				MaxUDPPayloadSize: d.config.Server.MaxUDPSize,
			}
			s = server.NewUDPServer(&serverConf)
		default:
			return fmt.Errorf("invalid bind protocol: %s", network)
		}

		go func() {
			err := s.ListenAndServe(d)
			select {
			case errChan <- err:
			default:
			}
		}()
	}

	listenerErr := <-errChan

	return fmt.Errorf("server listener failed and exited: %w", listenerErr)
}

func caPath2Pool(cas []string) (*x509.CertPool, error) {
	rootCAs := x509.NewCertPool()

	for _, ca := range cas {
		pem, err := ioutil.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("ReadFile: %w", err)
		}

		if ok := rootCAs.AppendCertsFromPEM(pem); !ok {
			return nil, fmt.Errorf("AppendCertsFromPEM: no certificate was successfully parsed in %s", ca)
		}
	}
	return rootCAs, nil
}
