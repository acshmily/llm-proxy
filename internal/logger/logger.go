package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

type Format int

const (
	TEXT Format = iota
	JSON
)

type Logger struct {
	mu     sync.Mutex
	level  Level
	format Format
}

func New(format Format, level Level) *Logger {
	return &Logger{format: format, level: level}
}

func (l *Logger) Info(msg string, fields ...LogField) {
	l.log(INFO, msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...LogField) {
	l.log(WARN, msg, fields...)
}

func (l *Logger) Error(msg string, fields ...LogField) {
	l.log(ERROR, msg, fields...)
}

type LogField struct {
	Key   string
	Value interface{}
}

func (l *Logger) log(level Level, msg string, fields ...LogField) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.format == JSON {
		entry := map[string]interface{}{
			"time":  time.Now().UTC().Format(time.RFC3339),
			"level": levelToString(level),
			"msg":   msg,
		}
		for _, f := range fields {
			entry[f.Key] = f.Value
		}
		data, _ := json.Marshal(entry)
		fmt.Fprintln(os.Stderr, string(data))
	} else {
		fmt.Fprintf(os.Stderr, "[%s] %s: %s\n",
			time.Now().Format(time.RFC3339),
			levelToString(level),
			msg)
	}
}

func levelToString(l Level) string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
