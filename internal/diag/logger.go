package diag

import (
	"log"
	"strings"
)

type Logger struct {
	base      *log.Logger
	debugMode bool
}

func New(base *log.Logger, level string) *Logger {
	return &Logger{
		base:      base,
		debugMode: strings.EqualFold(strings.TrimSpace(level), "debug"),
	}
}

func (l *Logger) Debugf(format string, args ...any) {
	if !l.debugMode {
		return
	}
	l.base.Printf("DEBUG "+format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.base.Printf("INFO "+format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.base.Printf("ERROR "+format, args...)
}

func (l *Logger) Fatalf(format string, args ...any) {
	l.base.Fatalf("FATAL "+format, args...)
}

func (l *Logger) IsDebug() bool {
	return l.debugMode
}
