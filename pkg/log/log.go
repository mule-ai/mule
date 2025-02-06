package log

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

const logFile = "dev-team.log"

func New(file string) logr.Logger {
	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	zc.DisableStacktrace = true
	// if you want logs on stdout us this line instead of the one below it
	// zc.OutputPaths = []string{"stdout", "tradestax.log"}
	if file == "" {
		file = logFile
	}
	zc.OutputPaths = []string{file}
	z, err := zc.Build()
	if err != nil {
		panic(err)
	}
	return zapr.NewLogger(z)
}

func NewStdoutLogger() logr.Logger {
	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	zc.DisableStacktrace = true
	zc.OutputPaths = []string{"stdout"}
	z, err := zc.Build()
	if err != nil {
		panic(err)
	}
	return zapr.NewLogger(z)
}
