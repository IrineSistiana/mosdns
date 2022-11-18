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

import "strings"

type RuleArgs struct {
	Matches []string `yaml:"matches"`
	Exec    string   `yaml:"exec"`
}

func parseArgs(ra RuleArgs) RuleConfig {
	var rc RuleConfig
	for _, s := range ra.Matches {
		rc.Matches = append(rc.Matches, parseMatch(s))
	}
	tag, typ, args := parseExec(ra.Exec)
	rc.Tag = tag
	rc.Type = typ
	rc.Args = args
	return rc
}

func parseMatch(s string) MatchConfig {
	var mc MatchConfig
	s = strings.TrimSpace(s)
	s, reverse := trimPrefixField(s, "!")
	mc.Reverse = reverse
	p, args, _ := strings.Cut(s, " ")
	args = strings.TrimSpace(args)
	mc.Args = args
	if tag, ok := trimPrefixField(p, "$"); ok {
		mc.Tag = tag
	} else {
		mc.Type = p
	}
	return mc
}

func parseExec(s string) (tag string, typ string, args string) {
	s = strings.TrimSpace(s)
	p, args, _ := strings.Cut(s, " ")
	args = strings.TrimSpace(args)
	p, ok := trimPrefixField(p, "$")
	if ok {
		tag = p
	} else {
		typ = p
	}
	return
}

type RuleConfig struct {
	Matches []MatchConfig `yaml:"matches"`
	Tag     string        `yaml:"tag"`
	Type    string        `yaml:"type"`
	Args    string        `yaml:"args"`
}

type MatchConfig struct {
	Tag     string `yaml:"tag"`
	Type    string `yaml:"type"`
	Args    string `yaml:"args"`
	Reverse bool   `yaml:"reverse"`
}

func trimPrefixField(s, p string) (string, bool) {
	if strings.HasPrefix(s, p) {
		return strings.TrimSpace(strings.TrimPrefix(s, p)), true
	}
	return s, false
}
