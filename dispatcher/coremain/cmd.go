package coremain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/ext_exec"
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

	buf := make([]byte, len(b))
	copy(buf, b)

	lh := make(intHeap, 0)
	i := 0
	for {
		if i == len(buf) {
			break
		}
		switch {
		case bytes.HasPrefix(buf[i:], []byte("${{")):
			lh.push(i)
			i++
		case bytes.HasPrefix(buf[i:], []byte("}}")):
			if li, ok := lh.pop(); ok {
				cmd := string(buf[li+3 : i])
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
				newBuf.Write(buf[:li])
				newBuf.Write(stdout)
				newBuf.Write(buf[i+2:])
				buf = newBuf.Bytes()
				i = li + 1
			} else {
				return nil, fmt.Errorf("unexpected }}: %s >>>> }} <<<< %s", string(buf[:i]), string(buf[i+2:]))
			}
		default:
			i++
		}
	}
	if li, ok := lh.pop(); ok {
		return nil, fmt.Errorf("unpaired ${{: %s >>>> ${{ <<<< %s", string(buf[:li]), string(buf[li+3:]))
	}

	return buf, nil
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
