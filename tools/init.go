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

package tools

import (
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/spf13/cobra"
)

func init() {
	probeCmd := &cobra.Command{
		Use:   "probe",
		Short: "Run some server tests.",
	}
	probeCmd.AddCommand(
		newConnReuseCmd(),
		newIdleTimeoutCmd(),
		newPipelineCmd(),
	)
	coremain.AddSubCmd(probeCmd)

	v2datCmd := &cobra.Command{
		Use:   "v2dat",
		Short: "Tools that can unpack v2ray data file to text files.",
	}
	v2datCmd.AddCommand(
		newUnpackDomainCmd(),
		newUnpackIPCmd(),
	)
	coremain.AddSubCmd(v2datCmd)

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Tools that can generate/convert mosdns config file.",
	}
	configCmd.AddCommand(newGenCmd(), newConvCmd())
	coremain.AddSubCmd(configCmd)
}
