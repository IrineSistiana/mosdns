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

package sequence

import (
	"context"
	"errors"
	"testing"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
)

type dummy struct {
	matched    bool
	wantErr    error
	wantR      *dns.Msg
	dropR      bool
	wantReturn bool
}

func (d *dummy) Match(ctx context.Context, qCtx *query_context.Context) (bool, error) {
	if d.wantErr != nil {
		return false, d.wantErr
	}
	return d.matched, nil
}

func (d *dummy) Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error {
	if d.wantErr != nil {
		return d.wantErr
	}
	if d.wantR != nil {
		qCtx.SetResponse(d.wantR)
	}
	if d.dropR {
		qCtx.SetResponse(nil)
	}
	if d.wantReturn {
		return nil
	}
	return next.ExecNext(ctx, qCtx)
}

func preparePlugins(p map[string]any) {
	p["target"] = &dummy{wantR: new(dns.Msg)}
	p["err"] = &dummy{wantErr: errors.New("err")}
	p["drop"] = &dummy{dropR: true}
	p["nop"] = &dummy{}
	p["true"] = &dummy{matched: true}
	p["false"] = &dummy{matched: false}
}

func Test_sequence_Exec(t *testing.T) {
	tests := []struct {
		name       string
		ra         []RuleArgs
		ra2        []RuleArgs
		wantErr    bool
		wantTarget bool
	}{
		{
			name: "exec",
			ra: []RuleArgs{
				{Exec: "$nop"},
				{Exec: "$target"},
				{Exec: "return"},
				{Exec: "$err"}, // skipped
			},
			wantErr:    false,
			wantTarget: true,
		},
		{
			name: "match",
			ra: []RuleArgs{
				{
					Matches: []string{"$true", "$false", "$err"}, // skip following matches when false
					Exec:    "$err",                              // skip exec when false
				},
				{
					Matches: []string{"$false", "$err"},
					Exec:    "$err",
				},
				{
					Matches: []string{"$true", "$true"},
					Exec:    "$target",
				},
			},
			wantErr:    false,
			wantTarget: true,
		},
		{
			name: "goto return",
			ra: []RuleArgs{
				{Exec: "goto seq2"},
				{Exec: "$err"}, // goto skips fallowing nodes.
			},
			ra2: []RuleArgs{
				{Exec: "$target"},
				{Exec: "return"},
				{Exec: "$err"}, // return skips fallowing nodes.
			},
			wantErr:    false,
			wantTarget: true,
		},
		{
			name: "jump return",
			ra: []RuleArgs{
				{Exec: "jump seq2"},
				{Exec: "$target"},
			},
			ra2: []RuleArgs{
				{Exec: "$nop"},
				{Exec: "return"},
				{Exec: "$err"},
			},
			wantErr:    false,
			wantTarget: true,
		},
		{
			name: "jump accept",
			ra: []RuleArgs{
				{Exec: "jump seq2"},
				{Exec: "$err"}, // accepted in seq2, skipped
			},
			ra2: []RuleArgs{
				{Exec: "$target"},
				{Exec: "accept"},
				{Exec: "$err"},
			},
			wantErr:    false,
			wantTarget: true,
		},
		{
			name: "jump end",
			ra: []RuleArgs{
				{Exec: "jump seq2"},
				{Exec: "$target"},
			},
			ra2: []RuleArgs{
				{Exec: "$nop"},
			},
			wantErr:    false,
			wantTarget: true,
		},
		{
			name: "reject",
			ra: []RuleArgs{
				{Exec: "reject"},
				{Exec: "$err"}, // skipped
			},
			wantErr:    false,
			wantTarget: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := make(map[string]any)
			m := coremain.NewTestMosdnsWithPlugins(ps)
			preparePlugins(ps)
			if len(tt.ra2) > 0 {
				s, err := NewSequence(coremain.NewBP("test", m), tt.ra2)
				if err != nil {
					t.Fatal(err)
				}
				ps["seq2"] = s
			}
			s, err := NewSequence(coremain.NewBP("test", m), tt.ra)
			if err != nil {
				t.Fatal(err)
			}
			qCtx := query_context.NewContext(new(dns.Msg))
			if err := s.Exec(context.Background(), qCtx); (err != nil) != tt.wantErr {
				t.Errorf("Exec() error = %v, wantErr %v", err, tt.wantErr)
			}
			if getTarget := qCtx.R() != nil; getTarget != tt.wantTarget {
				t.Errorf("Exec() getTarget = %v, wantTarget %v", getTarget, tt.wantTarget)
			}
		})
	}
}
