package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel уровень логирования
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

// Logger структура логгера
type Logger struct {
	mu       sync.Mutex
	level    LogLevel
	format   string // "json" или "text"
	output   *log.Logger
	fields   map[string]interface{}
}

// NewLogger создает новый логгер
func NewLogger(level, format string) *Logger {
	var logLevel LogLevel
	switch strings.ToLower(level) {
	case "debug":
		logLevel = DebugLevel
	case "info":
		logLevel = InfoLevel
	case "warn", "warning":
		logLevel = WarnLevel
	case "error":
		logLevel = ErrorLevel
	case "fatal":
		logLevel = FatalLevel
	default:
		logLevel = InfoLevel
	}

	return &Logger{
		level:  logLevel,
		format: format,
		output: log.New(os.Stdout, "", 0),
		fields: make(map[string]interface{}),
	}
}

// WithField добавляет поле к логгеру
func (l *Logger) WithField(key string, value interface{}) *Logger {
	newLogger := &Logger{
		level:  l.level,
		format: l.format,
		output: l.output,
		fields: make(map[string]interface{}),
	}
	
	// Копируем существующие поля
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value
	
	return newLogger
}

// WithFields добавляет несколько полей к логгеру
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := &Logger{
		level:  l.level,
		format: l.format,
		output: l.output,
		fields: make(map[string]interface{}),
	}
	
	// Копируем существующие поля
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	// Добавляем новые поля
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	
	return newLogger
}

// WithContext добавляет контекстную информацию
func (l *Logger) WithContext(ctx context.Context) *Logger {
	// TODO: Извлечь trace ID, user ID и другую информацию из контекста
	return l
}

// Debug логирует сообщение уровня debug
func (l *Logger) Debug(msg string) {
	l.log(DebugLevel, msg)
}

// Debugf логирует форматированное сообщение уровня debug
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DebugLevel, fmt.Sprintf(format, args...))
}

// Info логирует сообщение уровня info
func (l *Logger) Info(msg string) {
	l.log(InfoLevel, msg)
}

// Infof логирует форматированное сообщение уровня info
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(InfoLevel, fmt.Sprintf(format, args...))
}

// Warn логирует сообщение уровня warn
func (l *Logger) Warn(msg string) {
	l.log(WarnLevel, msg)
}

// Warnf логирует форматированное сообщение уровня warn
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WarnLevel, fmt.Sprintf(format, args...))
}

// Error логирует сообщение уровня error
func (l *Logger) Error(msg string) {
	l.log(ErrorLevel, msg)
}

// Errorf логирует форматированное сообщение уровня error
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ErrorLevel, fmt.Sprintf(format, args...))
}

// Fatal логирует сообщение уровня fatal и завершает программу
func (l *Logger) Fatal(msg string) {
	l.log(FatalLevel, msg)
	os.Exit(1)
}

// Fatalf логирует форматированное сообщение уровня fatal и завершает программу
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(FatalLevel, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// log выполняет логирование
func (l *Logger) log(level LogLevel, msg string) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Добавляем базовые поля
	fields := make(map[string]interface{})
	for k, v := range l.fields {
		fields[k] = v
	}
	
	fields["time"] = time.Now().Format(time.RFC3339)
	fields["level"] = levelString(level)
	fields["msg"] = msg
	
	// Добавляем информацию о вызывающем коде
	if l.level <= DebugLevel {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			fields["file"] = fmt.Sprintf("%s:%d", file, line)
		}
	}

	// Форматируем вывод
	if l.format == "json" {
		l.outputJSON(fields)
	} else {
		l.outputText(fields)
	}
}

// outputJSON выводит лог в JSON формате
func (l *Logger) outputJSON(fields map[string]interface{}) {
	// Простая JSON сериализация
	parts := make([]string, 0, len(fields))
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf(`"%s":"%v"`, k, v))
	}
	l.output.Printf("{%s}", strings.Join(parts, ","))
}

// outputText выводит лог в текстовом формате
func (l *Logger) outputText(fields map[string]interface{}) {
	// Формируем текстовое сообщение
	timestamp := fields["time"]
	level := fields["level"]
	msg := fields["msg"]
	
	// Базовый формат
	logMsg := fmt.Sprintf("[%s] %s %s", timestamp, level, msg)
	
	// Добавляем дополнительные поля
	extraFields := make([]string, 0)
	for k, v := range fields {
		if k != "time" && k != "level" && k != "msg" {
			extraFields = append(extraFields, fmt.Sprintf("%s=%v", k, v))
		}
	}
	
	if len(extraFields) > 0 {
		logMsg += " " + strings.Join(extraFields, " ")
	}
	
	l.output.Println(logMsg)
}

// levelString возвращает строковое представление уровня
func levelString(level LogLevel) string {
	switch level {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Default logger instance
var defaultLogger = NewLogger("info", "text")

// DefaultLogger экспортированная переменная для совместимости с broadcast.go
var DefaultLogger = defaultLogger

// SetDefaultLogger устанавливает логгер по умолчанию
func SetDefaultLogger(logger *Logger) {
	defaultLogger = logger
}

// Debug логирует сообщение уровня debug
func Debug(msg string) {
	defaultLogger.Debug(msg)
}

// Debugf логирует форматированное сообщение уровня debug
func Debugf(format string, args ...interface{}) {
	defaultLogger.Debugf(format, args...)
}

// Info логирует сообщение уровня info
func Info(msg string) {
	defaultLogger.Info(msg)
}

// Infof логирует форматированное сообщение уровня info
func Infof(format string, args ...interface{}) {
	defaultLogger.Infof(format, args...)
}

// Warn логирует сообщение уровня warn
func Warn(msg string) {
	defaultLogger.Warn(msg)
}

// Warnf логирует форматированное сообщение уровня warn
func Warnf(format string, args ...interface{}) {
	defaultLogger.Warnf(format, args...)
}

// Error логирует сообщение уровня error
func Error(msg string) {
	defaultLogger.Error(msg)
}

// Errorf логирует форматированное сообщение уровня error
func Errorf(format string, args ...interface{}) {
	defaultLogger.Errorf(format, args...)
}

// Fatal логирует сообщение уровня fatal и завершает программу
func Fatal(msg string) {
	defaultLogger.Fatal(msg)
}

// Fatalf логирует форматированное сообщение уровня fatal и завершает программу
func Fatalf(format string, args ...interface{}) {
	defaultLogger.Fatalf(format, args...)
}