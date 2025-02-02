// Package logger - пакет для логирования.
//
// Пока что представлен в виде печати в stderr.
package logger

import "log"

var defaultLogger Logger = &StderrLogger{
	logLevel: 3,
}

type Logger interface {
	Init()
	Log(msg string, values ...interface{})
	Panic(msg string, values ...interface{})
	Info(msg string, values ...interface{})
	Debug(msg string, values ...interface{})
	LogLevel() int
	SetLogLevel(level int)
}

type StderrLogger struct {
	logLevel int
}

func (l *StderrLogger) Init() {
	l.logLevel = 3
}

func (l *StderrLogger) Log(msg string, values ...interface{}) {
	log.Println(append([]interface{}{msg}, values...)...)
}

func (l *StderrLogger) Panic(msg string, values ...interface{}) {
	log.Panicln(append([]interface{}{msg}, values...)...)
}

func (l *StderrLogger) Info(msg string, values ...interface{}) {
	log.Println(append([]interface{}{msg}, values...)...)
}

func (l *StderrLogger) Debug(msg string, values ...interface{}) {
	log.Println(append([]interface{}{msg}, values...)...)
}

func (l *StderrLogger) LogLevel() int {
	return l.logLevel
}

func (l *StderrLogger) SetLogLevel(level int) {
	l.logLevel = level
}

// Log - сообщение с уровнем приоритета 3
func Log(msg string, values ...interface{}) {
	if defaultLogger.LogLevel() >= 3 {
		defaultLogger.Log(msg, values...)
	}
}

// Panic - сообщение с максимальным уровнем приоритета. Будет залогировано при любом значение уровня
func Panic(msg string, values ...interface{}) {
	defaultLogger.Panic(msg, values...)
}

// Info - сообщение с уровнем 3
func Info(msg string, values ...interface{}) {
	if defaultLogger.LogLevel() >= 3 {
		defaultLogger.Info(msg, values...)
	}
}

// Debug - сообщение с уровнем 5
func Debug(msg string, values ...interface{}) {
	if defaultLogger.LogLevel() >= 5 {
		defaultLogger.Debug(msg, values...)
	}
}

// SetLogger - замена логера по умолчанию
func SetLogger(logger Logger) {
	defaultLogger = logger
}

// SetLogLevel - изменения уровня логирования
func SetLogLevel(level int) {
	defaultLogger.SetLogLevel(level)
}
