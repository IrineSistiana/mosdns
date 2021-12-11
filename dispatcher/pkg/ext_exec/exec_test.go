package ext_exec

import (
	"bytes"
	"context"
	"testing"
)

func TestGetStringFromCmd(t *testing.T) {
	out, err := GetOutputFromCmd(context.Background(), "go version")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("go version go")) {
		t.Fatalf("unexpected output: [%s]", out)
	}
}
