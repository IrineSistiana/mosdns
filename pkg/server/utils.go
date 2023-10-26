package server

import (
	"errors"

	"go.uber.org/zap"
)

var (
	errListenerCtxCanceled   = errors.New("listener ctx canceled")
	errConnectionCtxCanceled = errors.New("connection ctx canceled")
)

var (
	nopLogger = zap.NewNop()
)
