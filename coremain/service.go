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

package coremain

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"time"
)

var (
	// initialized by "service" sub command
	svc    service.Service
	svcCfg = &service.Config{
		Name:        "mosdns",
		DisplayName: "mosdns",
		Description: "A DNS forwarder",
	}
)

type serverService struct {
	f *serverFlags
	m *Mosdns
}

func (ss *serverService) Start(s service.Service) error {
	mlog.L().Info("starting service", zap.String("platform", s.Platform()))
	m, err := NewServer(ss.f)
	if err != nil {
		return err
	}
	ss.m = m
	go func() {
		err := m.GetSafeClose().WaitClosed()
		if err != nil {
			m.Logger().Fatal("server exited", zap.Error(err))
		} else {
			m.Logger().Info("server exited")
		}
	}()
	return nil
}

func (ss *serverService) Stop(_ service.Service) error {
	ss.m.Logger().Info("service is shutting down")
	ss.m.GetSafeClose().SendCloseSignal(nil)
	return ss.m.GetSafeClose().WaitClosed()
}

// initService will init svc for sub command "service"
func initService(_ *cobra.Command, _ []string) error {
	s, err := service.New(&serverService{}, svcCfg)
	if err != nil {
		return fmt.Errorf("cannot init service, %w", err)
	}
	svc = s
	return nil
}

func newSvcInstallCmd() *cobra.Command {
	sf := new(serverFlags)
	c := &cobra.Command{
		Use:   "install [-d working_dir] [-c config_file]",
		Short: "Install mosdns as a system service.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(sf.dir) > 0 {
				absWd, err := filepath.Abs(sf.dir)
				if err != nil {
					return fmt.Errorf("cannot solve absolute working dir path, %w", err)
				} else {
					sf.dir = absWd
				}
			} else {
				ep, err := os.Executable()
				if err != nil {
					return fmt.Errorf("cannot solve current executable path, %w", err)
				}
				sf.dir = filepath.Dir(ep)
			}
			mlog.S().Infof("set service working dir as %s", sf.dir)
			svcCfg.Arguments = []string{"start", "--as-service", "-d", sf.dir}
			if len(sf.c) > 0 {
				svcCfg.Arguments = append(svcCfg.Arguments, "-c", sf.c)
			}
			return svc.Install()
		},
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
	}
	c.Flags().StringVarP(&sf.dir, "dir", "d", "", "working dir")
	c.Flags().StringVarP(&sf.c, "config", "c", "", "config path")
	return c
}

func newSvcUninstallCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall mosdns from system service.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return svc.Uninstall()
		},
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
	}
	return c
}

func newSvcStartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "start",
		Short: "Start mosdns system service.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.Start(); err != nil {
				return err
			}

			mlog.S().Info("service is starting")
			time.Sleep(time.Second)
			s, err := svc.Status()
			if err != nil {
				mlog.S().Warn("cannot get service status, %w", err)
			} else {
				switch s {
				case service.StatusRunning:
					mlog.S().Info("service is running")
				case service.StatusStopped:
					mlog.S().Error("service is stopped, check mosdns and system service log for more info")
				default:
					mlog.S().Warn("cannot get service status, system may not support this operation")
				}
			}

			return nil
		},
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
	}
	return c
}

func newSvcStopCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "stop",
		Short: "Stop mosdns system service.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return svc.Stop()
		},
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
	}
	return c
}

func newSvcRestartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "restart",
		Short: "Restart mosdns system service.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return svc.Restart()
		},
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
	}
	return c
}

func newSvcStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: "Status of mosdns system service.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := svc.Status()
			if err != nil {
				return fmt.Errorf("cannot get service status, %w", err)
			}
			var out string
			switch s {
			case service.StatusRunning:
				out = "running"
			case service.StatusStopped:
				out = "stopped"
			case service.StatusUnknown:
				out = "unknown"
			}
			println(out)
			return nil
		},
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
	}
	return c
}
