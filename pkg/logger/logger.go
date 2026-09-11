package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"runtime"
	"time"
)

type Level int8

type Fields map[string]any

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
	LevelPanic
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	case LevelPanic:
		return "PANIC"
	}
	return ""
}

type Logger struct {
	newLogger *log.Logger
	ctx       context.Context
	fields    Fields
	callers   []string
}

func NewLogger(w io.Writer) *Logger {
	l := log.New(w, "", 0)
	return &Logger{newLogger: l}
}

func (l *Logger) clone() *Logger {
	n1 := *l
	return &n1
}

func (l *Logger) WithFields(fields Fields) *Logger {
	ll := l.clone()
	ll.fields = make(Fields, len(l.fields)+len(fields))

	for k, v := range l.fields {
		ll.fields[k] = v
	}

	for k, v := range fields {
		ll.fields[k] = v
	}

	return ll
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	ll := l.clone()
	ll.ctx = ctx
	return ll
}

func (l *Logger) WithCaller(skip int) *Logger {
	ll := l.clone()
	pc, file, line, ok := runtime.Caller(skip)
	if ok {
		f := runtime.FuncForPC(pc)
		ll.callers = []string{fmt.Sprintf("%s:%d %s", file, line, f.Name())}
	}
	return ll
}

func (l *Logger) WithCallersFrames() *Logger {
	maxcallersDepth := 25
	minCallersDepth := 1
	callers := []string{}
	pcs := make([]uintptr, maxcallersDepth)
	depth := runtime.Callers(minCallersDepth, pcs)
	frames := runtime.CallersFrames(pcs[:depth])
	for {
		frame, more := frames.Next()

		s := fmt.Sprintf(
			"%s:%d %s",
			frame.File,
			frame.Line,
			frame.Function,
		)
		callers = append(callers, s)

		if !more {
			break
		}
	}

	ll := l.clone()
	ll.callers = callers
	return ll
}

func (l *Logger) JSONFormat(level Level, message string) map[string]any {
	data := make(Fields, len(l.fields)+4)
	data["level"] = level.String()
	data["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	data["message"] = message
	data["callers"] = l.callers
	if len(l.fields) > 0 {
		for k, v := range l.fields {
			if _, ok := data[k]; !ok {
				data[k] = v
			}
		}
	}
	return data
}

func (l *Logger) Output(level Level, message string) {
	body, err := json.Marshal(l.JSONFormat(level, message))
	if err != nil {
		l.newLogger.Printf(
			`{"level":"ERROR","message":"marshal log entry failed","error":%q}`,
			err.Error(),
		)
		return
	}
	content := string(body)
	switch level {
	case LevelDebug:
		l.newLogger.Print(content)
	case LevelInfo:
		l.newLogger.Print(content)
	case LevelWarn:
		l.newLogger.Print(content)
	case LevelError:
		l.newLogger.Print(content)
	case LevelFatal:
		l.newLogger.Fatal(content)
	case LevelPanic:
		l.newLogger.Panic(content)
	}
}

func (l *Logger) Info(v ...any) {
	l.Output(LevelInfo, fmt.Sprint(v...))
}

func (l *Logger) Infof(format string, v ...any) {
	l.Output(LevelInfo, fmt.Sprintf(format, v...))
}

func (l *Logger) Debug(v ...any) {
	l.Output(LevelDebug, fmt.Sprint(v...))
}

func (l *Logger) Debugf(format string, v ...any) {
	l.Output(LevelDebug, fmt.Sprintf(format, v...))
}

func (l *Logger) Warn(v ...any) {
	l.Output(LevelWarn, fmt.Sprint(v...))
}

func (l *Logger) Warnf(format string, v ...any) {
	l.Output(LevelWarn, fmt.Sprintf(format, v...))
}

func (l *Logger) Error(v ...any) {
	l.Output(LevelError, fmt.Sprint(v...))
}

func (l *Logger) Errorf(format string, v ...any) {
	l.Output(LevelError, fmt.Sprintf(format, v...))
}

func (l *Logger) Fatal(v ...any) {
	l.Output(LevelFatal, fmt.Sprint(v...))
}

func (l *Logger) Fatalf(format string, v ...any) {
	l.Output(LevelFatal, fmt.Sprintf(format, v...))
}

func (l *Logger) Panic(v ...any) {
	l.Output(LevelPanic, fmt.Sprint(v...))
}

func (l *Logger) Panicf(format string, v ...any) {
	l.Output(LevelPanic, fmt.Sprintf(format, v...))
}
