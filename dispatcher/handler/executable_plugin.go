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

package handler

import (
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"reflect"
	"strings"
	"sync"
)

type ExecutablePlugin interface {
	Plugin
	Executable
}

type Executable interface {
	Exec(ctx context.Context, qCtx *Context) (err error)
}

type ESExecutablePlugin interface {
	Plugin
	ESExecutable
}

// ESExecutable: Early Stoppable Executable.
// Which can stop the ExecutableCmdSequence immediately if earlyStop is true.
type ESExecutable interface {
	ExecES(ctx context.Context, qCtx *Context) (earlyStop bool, err error)
}

type ExecutableCmd interface {
	ExecCmd(ctx context.Context, qCtx *Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error)
}

type executablePluginTag struct {
	s string
}

func (t executablePluginTag) ExecCmd(ctx context.Context, qCtx *Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error) {
	p, err := GetPlugin(t.s)
	if err != nil {
		return "", false, err
	}

	logger.Debug("exec executable plugin", qCtx.InfoField(), zap.String("exec", t.s))
	switch {
	case p.Is(PITExecutable):
		return "", false, p.Exec(ctx, qCtx)
	case p.Is(PITESExecutable):
		earlyStop, err = p.ExecES(ctx, qCtx)
		return "", earlyStop, err
	default:
		return "", false, fmt.Errorf("plugin %s class err", t.s)
	}
}

type IfBlockConfig struct {
	If    []string      `yaml:"if"`
	IfAnd []string      `yaml:"if_and"`
	Exec  []interface{} `yaml:"exec"`
	Goto  string        `yaml:"goto"`
}

type matcher struct {
	tag    string
	negate bool
}

func paresMatcher(s []string) []matcher {
	m := make([]matcher, 0, len(s))
	for _, tag := range s {
		if strings.HasPrefix(tag, "!") {
			m = append(m, matcher{tag: strings.TrimPrefix(tag, "!"), negate: true})
		} else {
			m = append(m, matcher{tag: tag})
		}
	}
	return m
}

type ifBlock struct {
	ifMatcher     []matcher
	ifAndMatcher  []matcher
	executableCmd ExecutableCmd
	goTwo         string
}

func (b *ifBlock) ExecCmd(ctx context.Context, qCtx *Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error) {
	if len(b.ifMatcher) > 0 {
		If, err := ifCondition(ctx, qCtx, logger, b.ifMatcher, false)
		if err != nil {
			return "", false, err
		}
		if If == false {
			return "", false, nil // if case returns false, skip this block.
		}
	}

	if len(b.ifAndMatcher) > 0 {
		If, err := ifCondition(ctx, qCtx, logger, b.ifAndMatcher, true)
		if err != nil {
			return "", false, err
		}
		if If == false {
			return "", false, nil
		}
	}

	// exec
	if b.executableCmd != nil {
		goTwo, earlyStop, err = b.executableCmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return "", false, err
		}
		if len(goTwo) != 0 || earlyStop {
			return goTwo, earlyStop, nil
		}
	}

	// goto
	if len(b.goTwo) != 0 { // if block has a goto, return it
		return b.goTwo, false, nil
	}

	return "", false, nil
}

func ifCondition(ctx context.Context, qCtx *Context, logger *zap.Logger, p []matcher, isAnd bool) (ok bool, err error) {
	if len(p) == 0 {
		return false, err
	}

	for _, m := range p {
		mp, err := GetPlugin(m.tag)
		if err != nil {
			return false, err
		}
		matched, err := mp.Match(ctx, qCtx)
		if err != nil {
			return false, err
		}
		logger.Debug("exec matcher plugin", qCtx.InfoField(), zap.String("exec", m.tag), zap.Bool("result", matched))

		res := matched != m.negate
		if !isAnd && res == true {
			return true, nil // or: if one of the case is true, skip others.
		}
		if isAnd && res == false {
			return false, nil // and: if one of the case is false, skip others.
		}

		ok = res
	}
	return ok, nil
}

func ParseIfBlock(in map[string]interface{}) (*ifBlock, error) {
	c := new(IfBlockConfig)
	err := WeakDecode(in, c)
	if err != nil {
		return nil, err
	}

	b := &ifBlock{
		ifMatcher:    paresMatcher(c.If),
		ifAndMatcher: paresMatcher(c.IfAnd),
		goTwo:        c.Goto,
	}

	if len(c.Exec) != 0 {
		ecs, err := ParseExecutableCmdSequence(c.Exec)
		if err != nil {
			return nil, err
		}
		b.executableCmd = ecs
	}

	return b, nil
}

type ParallelECS struct {
	s []*ExecutableCmdSequence
}

type ParallelECSConfig struct {
	Parallel [][]interface{} `yaml:"parallel"`
}

func ParseParallelECS(in [][]interface{}) (*ParallelECS, error) {
	ps := make([]*ExecutableCmdSequence, 0, len(in))
	for i, subSequence := range in {
		es, err := ParseExecutableCmdSequence(subSequence)
		if err != nil {
			return nil, fmt.Errorf("invalid parallel sequence at index %d: %w", i, err)
		}
		ps = append(ps, es)
	}
	return &ParallelECS{s: ps}, nil
}

type parallelResult struct {
	qCtx *Context
	err  error
	from int
}

func (p *ParallelECS) ExecCmd(ctx context.Context, qCtx *Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error) {
	return "", false, p.execCmd(ctx, qCtx, logger)
}

func (p *ParallelECS) execCmd(ctx context.Context, qCtx *Context, logger *zap.Logger) (err error) {
	t := len(p.s)
	switch t {
	case 0:
		return nil
	case 1:
		return WalkExecutableCmd(ctx, qCtx, logger, p.s[0])
	}

	c := make(chan *parallelResult, t) // use buf chan to avoid block.
	for i, sequence := range p.s {
		i := i
		sequence := sequence
		qCtxCopy := qCtx.Copy()
		go func() {
			err := WalkExecutableCmd(ctx, qCtxCopy, logger, sequence)
			c <- &parallelResult{
				qCtx: qCtxCopy,
				err:  err,
				from: i,
			}
		}()
	}

	for i := 0; i < t; i++ {
		select {
		case res := <-c:
			if res.err != nil {
				logger.Warn("sequence failed", qCtx.InfoField(), zap.Int("sequence_index", res.from), zap.Error(res.err))
				continue
			}
			if res.qCtx.R() == nil {
				logger.Debug("sequence returned with an empty response", qCtx.InfoField(), zap.Int("sequence_index", res.from))
				continue
			}

			logger.Debug("sequence returned a response", qCtx.InfoField(), zap.Int("sequence_index", res.from))
			qCtx.SetResponse(res.qCtx.R(), res.qCtx.Status())
			return nil

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// No valid response, all parallel sequences are failed.
	qCtx.SetResponse(nil, ContextStatusServerFailed)
	return errors.New("no response")
}

type FallbackConfig struct {
	// Primary exec sequence, must have at least one element.
	Primary []interface{} `yaml:"primary"`
	// Secondary exec sequence, must have at least one element.
	Secondary []interface{} `yaml:"secondary"`

	StatLength int `yaml:"stat_length"` // default is 10
	Threshold  int `yaml:"threshold"`   // default is 5
}

type FallbackECS struct {
	primary   *ExecutableCmdSequence
	secondary *ExecutableCmdSequence
	threshold int

	primaryST *statusTracker
}

type statusTracker struct {
	sync.Mutex
	threshold int
	status    []uint8 // 0 means success, !0 means failed
	p         int
}

func newStatusTracker(threshold, statLength int) *statusTracker {
	return &statusTracker{
		threshold: threshold,
		status:    make([]uint8, statLength),
	}
}

func (t *statusTracker) good() bool {
	t.Lock()
	defer t.Unlock()

	var failedSum int
	for _, s := range t.status {
		if s != 0 {
			failedSum++
		}
	}
	return failedSum < t.threshold
}

func (t *statusTracker) update(s uint8) {
	t.Lock()
	defer t.Unlock()

	if t.p >= len(t.status) {
		t.p = 0
	}
	t.status[t.p] = s
	t.p++
}

func ParseFallbackECS(primary, secondary []interface{}, threshold, statLength int) (*FallbackECS, error) {
	primaryECS, err := ParseExecutableCmdSequence(primary)
	if err != nil {
		return nil, fmt.Errorf("invalid primary sequence: %w", err)
	}

	secondaryECS, err := ParseExecutableCmdSequence(secondary)
	if err != nil {
		return nil, fmt.Errorf("invalid secondary sequence: %w", err)
	}

	if threshold > statLength {
		threshold = statLength
	}
	if statLength <= 0 {
		statLength = 10
	}
	if threshold <= 0 {
		threshold = 5
	}

	return &FallbackECS{
		primary:   primaryECS,
		secondary: secondaryECS,
		threshold: threshold,
		primaryST: newStatusTracker(threshold, statLength),
	}, nil
}

func (f *FallbackECS) ExecCmd(ctx context.Context, qCtx *Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error) {
	return "", false, f.execCmd(ctx, qCtx, logger)
}

func (f *FallbackECS) execCmd(ctx context.Context, qCtx *Context, logger *zap.Logger) (err error) {
	switch {
	case f.primary.Len()+f.secondary.Len() == 0:
		return nil
	case f.primary.Len() != 0 && f.secondary.Len() == 0:
		return WalkExecutableCmd(ctx, qCtx, logger, f.primary)
	case f.primary.Len() == 0 && f.secondary.Len() != 0:
		return WalkExecutableCmd(ctx, qCtx, logger, f.secondary)
	}

	if f.primaryST.good() {
		return f.execPrimary(ctx, qCtx, logger)
	}
	logger.Debug("primary is not good", qCtx.InfoField())
	return f.doFallback(ctx, qCtx, logger)
}

func (f *FallbackECS) execPrimary(ctx context.Context, qCtx *Context, logger *zap.Logger) (err error) {
	err = WalkExecutableCmd(ctx, qCtx, logger, f.primary)
	if err != nil || qCtx.R() == nil {
		f.primaryST.update(1)
	} else {
		f.primaryST.update(0)
	}
	return err
}

type fallbackResult struct {
	qCtx *Context
	err  error
	from string
}

func (f *FallbackECS) doFallback(ctx context.Context, qCtx *Context, logger *zap.Logger) (err error) {
	c := make(chan *fallbackResult, 2) // buf size is 2, avoid block.

	qCtxCopyP := qCtx.Copy()
	go func() {
		err := f.execPrimary(ctx, qCtxCopyP, logger)
		c <- &fallbackResult{
			qCtx: qCtxCopyP,
			err:  err,
			from: "primary",
		}
	}()

	qCtxCopyS := qCtx.Copy()
	go func() {
		err := WalkExecutableCmd(ctx, qCtxCopyS, logger, f.secondary)
		c <- &fallbackResult{
			qCtx: qCtxCopyS,
			err:  err,
			from: "secondary",
		}
	}()

	for i := 0; i < 2; i++ {
		select {
		case res := <-c:
			if res.err != nil {
				logger.Warn("sequence failed", qCtx.InfoField(), zap.String("sequence", res.from), zap.Error(err))
				continue
			}

			if res.qCtx.R() == nil {
				logger.Debug("sequence returned with an empty response ", qCtx.InfoField(), zap.String("sequence", res.from))
				continue
			}

			logger.Debug("sequence returned a response", qCtx.InfoField(), zap.String("sequence", res.from))
			qCtx.SetResponse(res.qCtx.R(), res.qCtx.Status())
			return nil

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// No response
	qCtx.SetResponse(nil, ContextStatusServerFailed)
	return errors.New("no response")
}

type ExecutableCmdSequence struct {
	c []ExecutableCmd
}

func ParseExecutableCmdSequence(in []interface{}) (*ExecutableCmdSequence, error) {
	es := &ExecutableCmdSequence{c: make([]ExecutableCmd, 0, len(in))}
	for i, v := range in {
		ec, err := parseExecutableCmd(v)
		if err != nil {
			return nil, fmt.Errorf("invalid cmd #%d: %w", i, err)
		}
		es.c = append(es.c, ec)
	}
	return es, nil
}

func parseExecutableCmd(in interface{}) (ExecutableCmd, error) {
	switch v := in.(type) {
	case string:
		return &executablePluginTag{s: v}, nil
	case map[string]interface{}:
		switch {
		case hasKey(v, "if") || hasKey(v, "if_and"): // if block
			ec, err := ParseIfBlock(v)
			if err != nil {
				return nil, fmt.Errorf("invalid if section: %w", err)
			}
			return ec, nil
		case hasKey(v, "parallel"): // parallel
			ec, err := parseParallelECS(v)
			if err != nil {
				return nil, fmt.Errorf("invalid parallel section: %w", err)
			}
			return ec, nil
		case hasKey(v, "primary"):
			ec, err := parseFallbackECS(v)
			if err != nil {
				return nil, fmt.Errorf("invalid fallback section: %w", err)
			}
			return ec, nil
		default:
			return nil, errors.New("unknown section")
		}
	default:
		return nil, fmt.Errorf("unexpected type: %s", reflect.TypeOf(in).String())
	}
}

func parseParallelECS(m map[string]interface{}) (ec ExecutableCmd, err error) {
	conf := new(ParallelECSConfig)
	err = WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	return ParseParallelECS(conf.Parallel)
}

func parseFallbackECS(m map[string]interface{}) (ec ExecutableCmd, err error) {
	conf := new(FallbackConfig)
	err = WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	return ParseFallbackECS(conf.Primary, conf.Secondary, conf.Threshold, conf.StatLength)
}

func hasKey(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}

// ExecCmd executes the sequence.
func (es *ExecutableCmdSequence) ExecCmd(ctx context.Context, qCtx *Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error) {
	for _, cmd := range es.c {
		goTwo, earlyStop, err = cmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return "", false, err
		}
		if len(goTwo) != 0 || earlyStop {
			return goTwo, earlyStop, nil
		}
	}

	return "", false, nil
}

func (es *ExecutableCmdSequence) Len() int {
	return len(es.c)
}

// WalkExecutableCmd executes the sequence, include its `goto`.
func WalkExecutableCmd(ctx context.Context, qCtx *Context, logger *zap.Logger, entry ExecutableCmd) (err error) {
	goTwo, _, err := entry.ExecCmd(ctx, qCtx, logger)
	if err != nil {
		return err
	}

	if len(goTwo) != 0 {
		logger.Debug("goto plugin", qCtx.InfoField(), zap.String("goto", goTwo))
		p, err := GetPlugin(goTwo)
		if err != nil {
			return err
		}
		return p.Exec(ctx, qCtx)
	}
	return nil
}
