/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package data_provider

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/safe_close"
	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type DataManager struct {
	pm sync.RWMutex
	ps map[string]*DataProvider
}

type DataListener interface {
	Update(newData []byte) error
}

func NewDataManager() *DataManager {
	return &DataManager{
		ps: make(map[string]*DataProvider),
	}
}

func (m *DataManager) AddDataProvider(name string, p *DataProvider) {
	m.pm.Lock()
	defer m.pm.Unlock()
	m.ps[name] = p
}

func (m *DataManager) GetDataProvider(name string) *DataProvider {
	m.pm.RLock()
	defer m.pm.RUnlock()
	return m.ps[name]
}

type DataProviderConfig struct {
	Tag        string `yaml:"tag"`
	File       string `yaml:"file"`
	AutoReload bool   `yaml:"auto_reload"`
}

type DataProvider struct {
	logger     *zap.Logger
	file       string
	autoReload bool

	lm        sync.Mutex
	listeners map[DataListener]struct{}

	sc *safe_close.SafeClose

	listenerPool sync.Pool
}

func NewDataProvider(lg *zap.Logger, cfg DataProviderConfig) (*DataProvider, error) {
	dp := &DataProvider{
		logger:     lg,
		file:       cfg.File,
		autoReload: cfg.AutoReload,
		listeners:  make(map[DataListener]struct{}),
		sc:         safe_close.NewSafeClose(),
		listenerPool: sync.Pool{
			New: func() interface{} {
				return make([]DataListener, 0, 8) // 预分配容量
			},
		},
	}

	if err := dp.init(); err != nil {
		return nil, err
	}
	return dp, nil
}

func (ds *DataProvider) init() error {
	_, err := ds.loadFromDisk()
	if err != nil {
		return err
	}

	if ds.autoReload {
		if err := ds.startFsWatcher(); err != nil {
			return fmt.Errorf("failed to start fs watcher, %w", err)
		}
	}
	return nil
}

func (ds *DataProvider) Close() {
	ds.sc.Done()
	ds.sc.CloseWait()
}

// LoadAndAddListener loads the DataListener, returns any error that occurs, and
// add this DataListener to this DataProvider.
func (ds *DataProvider) LoadAndAddListener(l DataListener) error {
	b, err := ds.GetData()
	if err != nil {
		return err
	}

	if err := l.Update(b); err != nil {
		return err
	}

	ds.lm.Lock()
	ds.listeners[l] = struct{}{}
	ds.lm.Unlock()
	return nil
}

func (ds *DataProvider) DeleteListener(l DataListener) {
	ds.lm.Lock()
	delete(ds.listeners, l)
	ds.lm.Unlock()
}

func (ds *DataProvider) GetData() ([]byte, error) {
	return os.ReadFile(ds.file)
}

// pushData notify the notifier and trigger all listeners.
func (ds *DataProvider) pushData(newData []byte) {
	ds.lm.Lock()
	ls := ds.listenerPool.Get().([]DataListener)
	ls = ls[:0] // 重置切片长度
	for listener := range ds.listeners {
		ls = append(ls, listener)
	}
	ds.lm.Unlock()

	for _, l := range ls {
		if err := l.Update(newData); err != nil {
			ds.logger.Error(
				"failed to update data listener",
				zap.Error(err),
			)
		}
	}

	ds.listenerPool.Put(ls) // 将切片放回池中
}

func (ds *DataProvider) loadFromDisk() ([]byte, error) {
	return os.ReadFile(ds.file)
}

func (ds *DataProvider) startFsWatcher() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(ds.file); err != nil {
		return err
	}

	go func() {
		defer w.Close()

		var delayReloadTimer *time.Timer
		for {
			select {
			case e, ok := <-w.Events:
				if !ok {
					return
				}
				ds.logger.Info(
					"fs event",
					zap.Stringer("event", e.Op),
					zap.String("file", e.Name),
				)

				if delayReloadTimer != nil {
					delayReloadTimer.Stop()
				}
				delayReloadTimer = time.AfterFunc(time.Second, func() {
					if hasOp(e, fsnotify.Remove) {
						_ = w.Remove(ds.file)
						if err := w.Add(ds.file); err != nil {
							ds.logger.Error(
								"failed to re-watch file, auto reload may not work anymore",
								zap.String("file", ds.file),
								zap.Error(err),
							)
						}
					}

					ds.logger.Info(
						"reloading file",
						zap.String("file", ds.file),
					)
					if v, err := ds.loadFromDisk(); err != nil {
						ds.logger.Error(
							"failed to reload file",
							zap.String("file", ds.file),
							zap.Error(err),
						)
					} else {
						ds.logger.Info(
							"file reloaded",
							zap.String("file", ds.file),
						)
						ds.pushData(v)
					}
				})

			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				ds.logger.Error("fs notify error", zap.Error(err))
			case <-ds.sc.ReceiveCloseSignal():
				return
			}
		}
	}()
	return nil
}

func hasOp(e fsnotify.Event, op fsnotify.Op) bool {
	return e.Op&op == op
}