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

package ext_exec

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GetOutputFromCmd exec the cmd and return the combined output.
func GetOutputFromCmd(ctx context.Context, cmd string) ([]byte, error) {
	ss := strings.Split(cmd, " ")
	name := ss[0]
	args := ss[1:]
	c := exec.CommandContext(ctx, name, args...)
	out, err := c.CombinedOutput()
	if err != nil {
		if len(out) != 0 {
			return nil, fmt.Errorf("cmd err: %w, output: %s", err, string(out))
		}
		return nil, err
	}

	return out, nil
}
