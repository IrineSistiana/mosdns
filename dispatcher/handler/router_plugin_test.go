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

package handler

import (
	"context"
	"testing"
)

func TestWalk(t *testing.T) {
	PurgePluginRegister()
	defer PurgePluginRegister()

	tests := []struct {
		name     string
		entryTag string
		wantErr  bool
	}{
		{"normal exec sequence 1", "p1", false},
		{"normal exec sequence 2", "end1", false},
		{"endless loop exec sequence", "e1", true},
		{"err exec sequence", "err1", true},
	}

	// add a normal exec sequence
	pluginTagRegister.register["p1"] = &dummySequencePlugin{next: "p2"}
	pluginTagRegister.register["p2"] = &dummySequencePlugin{next: "p3"}
	pluginTagRegister.register["p3"] = &dummySequencePlugin{next: ""} // the end

	pluginTagRegister.register["end1"] = &dummySequencePlugin{next: StopSignTag} // the end

	// add a endless loop exec sequence
	pluginTagRegister.register["e1"] = &dummySequencePlugin{next: "e2"}
	pluginTagRegister.register["e2"] = &dummySequencePlugin{next: "e3"}
	pluginTagRegister.register["e3"] = &dummySequencePlugin{next: "e1"} // endless loop

	// add a exec sequence which raise an err
	pluginTagRegister.register["err1"] = &dummySequencePlugin{next: "err2"}
	pluginTagRegister.register["err2"] = &dummySequencePlugin{next: "err3", hasErr: true}
	pluginTagRegister.register["err3"] = &dummySequencePlugin{next: "", shouldNoTBeReached: true}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Walk(ctx, nil, tt.entryTag); (err != nil) != tt.wantErr {
				t.Errorf("Walk() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
