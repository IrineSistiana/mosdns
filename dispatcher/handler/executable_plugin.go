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
	"fmt"
	"github.com/sirupsen/logrus"
	"reflect"
	"strings"
)

type ExecutablePlugin interface {
	Plugin
	Executable
}

type Executable interface {
	Exec(ctx context.Context, qCtx *Context) (err error)
}

type ExecutablePluginWrapper struct {
	tag string
	typ string

	Executable
}

func (p *ExecutablePluginWrapper) Tag() string {
	return p.tag
}

func (p *ExecutablePluginWrapper) Type() string {
	return p.typ
}

// WrapExecutablePlugin returns a *ExecutablePluginWrapper which implements Plugin and ExecutablePlugin.
func WrapExecutablePlugin(tag, typ string, executable Executable) *ExecutablePluginWrapper {
	return &ExecutablePluginWrapper{
		tag:        tag,
		typ:        typ,
		Executable: executable,
	}
}

type ExecutableCmd interface {
	ExecCmd(ctx context.Context, qCtx *Context, logger *logrus.Entry) (goTwo string, err error)
}

type executablePluginTag string

func (tag executablePluginTag) ExecCmd(ctx context.Context, qCtx *Context, logger *logrus.Entry) (goTwo string, err error) {
	p, err := GetExecutablePlugin(string(tag))
	if err != nil {
		return "", err
	}
	logger.Debugf("%v: exec plugin %s", qCtx, tag)
	return "", p.Exec(ctx, qCtx)
}

type IfBlockConfig struct {
	If   []string      `yaml:"if"`
	Exec []interface{} `yaml:"exec"`
	Goto string        `yaml:"goto"`
}

type ifBlock struct {
	ifMather      []string
	executableCmd ExecutableCmd
	goTwo         string
}

func (b *ifBlock) ExecCmd(ctx context.Context, qCtx *Context, logger *logrus.Entry) (goTwo string, err error) {
	If := true
	for _, tag := range b.ifMather {
		if len(tag) == 0 {
			continue
		}

		reverse := false
		if reverse = strings.HasPrefix(tag, "!"); reverse {
			tag = strings.TrimPrefix(tag, "!")
		}

		m, err := GetMatcherPlugin(tag)
		if err != nil {
			return "", err
		}
		matched, err := m.Match(ctx, qCtx)
		if err != nil {
			return "", err
		}
		logger.Debugf("%v: exec matcher plugin %s, returned: %v", qCtx, tag, matched)

		If = matched != reverse
		if If == true {
			break // if one of the case is true, skip others.
		}
	}
	if If == false {
		return "", nil // if case returns false, skip this block.
	}

	// exec
	if b.executableCmd != nil {
		goTwo, err = b.executableCmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return "", err
		}
		if len(goTwo) != 0 {
			return goTwo, nil
		}
	}

	// goto
	if len(b.goTwo) != 0 { // if block has a goto, return it
		return b.goTwo, nil
	}

	return "", nil
}

type ExecutableCmdSequence []ExecutableCmd

func NewExecutableCmdSequence() *ExecutableCmdSequence {
	return new(ExecutableCmdSequence)
}

func (es *ExecutableCmdSequence) Parse(in []interface{}) error {
	for i := range in {
		switch v := in[i].(type) {
		case string:
			*es = append(*es, executablePluginTag(v))
		case map[string]interface{}:
			c := new(IfBlockConfig)
			err := WeakDecode(v, c)
			if err != nil {
				return err
			}

			ifBlock := &ifBlock{
				ifMather: c.If,
				goTwo:    c.Goto,
			}

			if len(c.Exec) != 0 {
				ecs := NewExecutableCmdSequence()
				err := ecs.Parse(c.Exec)
				if err != nil {
					return err
				}
				ifBlock.executableCmd = ecs
			}

			*es = append(*es, ifBlock)
		default:
			return fmt.Errorf("unexpected type: %s", reflect.TypeOf(in[i]).String())
		}
	}
	return nil
}

// ExecCmd executes the sequence.
func (es *ExecutableCmdSequence) ExecCmd(ctx context.Context, qCtx *Context, logger *logrus.Entry) (goTwo string, err error) {
	for _, cmd := range *es {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		goTwo, err = cmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return "", err
		}
		if len(goTwo) != 0 {
			return goTwo, nil
		}
	}

	return "", nil
}

// Exec executes the sequence, include its `goto`.
func (es *ExecutableCmdSequence) Exec(ctx context.Context, qCtx *Context, logger *logrus.Entry) (err error) {
	goTwo, err := es.ExecCmd(ctx, qCtx, logger)
	if err != nil {
		return err
	}

	if len(goTwo) != 0 {
		logger.Debugf("%v: goto plugin %s", qCtx, goTwo)
		p, err := GetExecutablePlugin(goTwo)
		if err != nil {
			return err
		}
		return p.Exec(ctx, qCtx)
	}
	return nil
}
