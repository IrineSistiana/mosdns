package coremain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/ext_exec"
	"go.uber.org/zap"
	"strings"
)

func runCmd(cmd string) ([]byte, error) {
	mlog.L().Info("executing config cmd", zap.String("cmd", cmd))
	stdout, err := ext_exec.GetOutputFromCmd(context.Background(), cmd)
	if err != nil {
		return nil, fmt.Errorf("cmd [%s] failed with err: %w", cmd, err)
	}
	stdout = bytes.Trim(stdout, "\r\n")
	mlog.L().Info("successfully executed cmd", zap.String("cmd", cmd), zap.String("stdout", string(stdout)))
	return stdout, nil
}

const (
	maxReplaceDepth = 16
)

var errReplaceDepth = errors.New("max replace depth reached")

func findAndReplaceCmd(b []byte, depth int, handleCmd func(string) ([]byte, error)) ([]byte, error) {
	if depth > maxReplaceDepth {
		return nil, errReplaceDepth
	}
	depth++

	leftIndicator := []byte("${{")
	rightIndicator := []byte("}}")
	buf := new(bytes.Buffer)

	remainData := b
	for len(remainData) > 0 {
		nextNewLineIdx := bytes.IndexByte(remainData, '\n')
		var line []byte
		if nextNewLineIdx == -1 {
			line = remainData
			remainData = remainData[0:0]
		} else {
			line = remainData[:nextNewLineIdx+1]
			remainData = remainData[nextNewLineIdx+1:]
		}

		lh := make(intHeap, 0)
		i := 0
	scanFor:
		for {
			if i >= len(line) {
				break
			}
			switch {
			case line[i] == '#':
				// This line starts with '#' or has a # separated from other
				// tokens by white space characters. Ignore it.
				if i == 0 || line[i-1] == ' ' {
					break scanFor
				}
				i++
			case bytes.HasPrefix(line[i:], leftIndicator):
				lh.push(i)
				i++
			case bytes.HasPrefix(line[i:], rightIndicator):
				if li, ok := lh.pop(); ok {
					cmd := string(line[li+3 : i])
					cmd = strings.Trim(cmd, " ")
					stdout, err := handleCmd(cmd)
					if err != nil {
						return nil, err
					}
					stdout, err = findAndReplaceCmd(stdout, depth, handleCmd)
					if err != nil {
						return nil, err
					}
					newBuf := new(bytes.Buffer)
					newBuf.Write(line[:li])
					newBuf.Write(stdout)
					newBuf.Write(line[i+2:])
					line = newBuf.Bytes()
					i = li + len(stdout)
				} else {
					return nil, fmt.Errorf("unexpected }}: %s >>>>}}<<<< %s", string(line[:i]), string(line[i+2:]))
				}
			default:
				i++
			}
		}
		if li, ok := lh.pop(); ok {
			return nil, fmt.Errorf("unpaired ${{: %s >>>>${{<<<< %s", string(line[:li]), string(line[li+3:]))
		}
		buf.Write(line)
	}

	return buf.Bytes(), nil
}

type intHeap []int

func (h intHeap) len() int { return len(h) }

func (h *intHeap) push(x int) {
	*h = append(*h, x)
}

func (h *intHeap) pop() (int, bool) {
	if len(*h) == 0 {
		return 0, false
	}

	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x, true
}
