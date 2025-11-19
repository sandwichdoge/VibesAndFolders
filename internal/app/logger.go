package app

import (
	"fmt"
	"log"
)

type Logger struct {
	debugEnabled bool
}

func NewLogger(debugEnabled bool) *Logger {
	return &Logger{debugEnabled: debugEnabled}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.debugEnabled {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func (l *Logger) Info(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

func (l *Logger) DebugSection(title string, content string) {
	if l.debugEnabled {
		fmt.Printf("=== %s ===\n%s\n%s\n", title, content, repeat("=", len(title)+8))
	}
}

func repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

var DefaultLogger = NewLogger(true)
