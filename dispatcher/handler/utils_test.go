package handler

import (
	"context"
	"testing"
)

func TestNode(t *testing.T) {
	var innerNode ExecutableChainNode
	chain := make([]ExecutableChainNode, 0)
	for i := 0; i < 10; i++ {
		innerNode = WrapExecutable(&dummyExecutable{})
		if i > 0 {
			pn := chain[i-1]
			pn.LinkNext(innerNode)
			innerNode.LinkPrevious(pn)
		}
		chain = append(chain, innerNode)
	}

	for _, cn := range chain {
		if FirstNode(cn) != chain[0] {
			t.Fatal()
		}
		if LatestNode(cn) != chain[len(chain)-1] {
			t.Fatal()
		}
	}
}

type dummyExecutable struct{}

func (d *dummyExecutable) Exec(_ context.Context, _ *Context, _ ExecutableChainNode) error {
	return nil
}
