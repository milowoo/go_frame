package lgames

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// LevelError ...
const (
	LevelError         = 1
	LevelWarning       = 2
	LevelInformational = 3
	LevelDebug         = 4
)

// Logger ...
type Logger struct {
	level int
	err   *log.Logger
	warn  *log.Logger
	info  *log.Logger
	debug *log.Logger
	depth int
}

// NewLogger ...
func NewLogger(flag int, numWorkers int, jobQueueLen int, depth int) *Logger {
	Logger := NewLogger3(os.Stdout, flag, numWorkers, jobQueueLen, depth)
	return Logger
}

// NewLogger2 ...
func NewLogger2(lfn string, maxsize int, flag int, numWorkers int, jobQueueLen int, depth int) *Logger {
	jack := &lumberjack.Logger{
		Filename: lfn,
		MaxSize:  maxsize, // megabytes
	}

	logger := NewLogger3(jack, flag, numWorkers, jobQueueLen, depth)

	return logger
}

// NewLogger3 ...
func NewLogger3(w io.Writer, flag int, numWorkers int, jobQueueLen int, depth int) *Logger {
	logger := new(Logger)
	logger.depth = depth
	if logger.depth <= 0 {
		logger.depth = 2
	}

	logger.err = log.New(w, "[ERROR] ", flag)
	logger.warn = log.New(w, "[WARNING] ", flag)
	logger.info = log.New(w, "[INFO] ", flag)
	logger.debug = log.New(w, "[DEBUG] ", flag)

	logger.SetLevel(LevelInformational)

	return logger
}

// SetLevel ...
func (ll *Logger) SetLevel(l int) int {
	ll.level = l
	return ll.level
}

func (ll *Logger) ResetLevel(level string) {
	switch strings.ToLower(level) {
	case "debug":
		ll.SetLevel(LevelDebug)
	case "info":
		ll.SetLevel(LevelInformational)
	case "warn":
		ll.SetLevel(LevelWarning)
	case "error":
		ll.SetLevel(LevelError)
	default:
		ll.SetLevel(LevelInformational)
	}
}

// GetLevel ...
func (ll *Logger) GetLevel() string {
	switch ll.level {
	case LevelDebug:
		return "Debug"
	case LevelError:
		return "Error"
	case LevelInformational:
		return "Info"
	case LevelWarning:
		return "Warn"
	}
	return ""
}

// 统一设置日志前缀
func (ll *Logger) SetPrefix(prefix string) {
	ll.err.SetPrefix(prefix)
	ll.warn.SetPrefix(prefix)
	ll.info.SetPrefix(prefix)
	ll.debug.SetPrefix(prefix)
}

// Error ...
func (ll *Logger) Error(format string, v ...interface{}) {
	if LevelError > ll.level {
		return
	}
	ll.err.Output(ll.depth, fmt.Sprintf(format, v...))
}

// Warn ...
func (ll *Logger) Warn(format string, v ...interface{}) {
	if LevelWarning > ll.level {
		return
	}
	ll.warn.Output(ll.depth, fmt.Sprintf(format, v...))
}

// Info ...
func (ll *Logger) Info(format string, v ...interface{}) {
	if LevelInformational > ll.level {
		return
	}
	ll.info.Output(ll.depth, fmt.Sprintf(format, v...))
}

// Debug ...
func (ll *Logger) Debug(format string, v ...interface{}) {
	if LevelDebug > ll.level {
		return
	}
	ll.debug.Output(ll.depth, fmt.Sprintf(format, v...))
}

// SetJack ...
func (ll *Logger) SetJack(lfn string, maxsize int) {
	jack := &lumberjack.Logger{
		Filename: lfn,
		MaxSize:  maxsize, // megabytes
	}

	ll.err.SetOutput(jack)
	ll.warn.SetOutput(jack)
	ll.info.SetOutput(jack)
	ll.debug.SetOutput(jack)
}

// SetFlag ...
func (ll *Logger) SetFlag(flag int) {
	ll.err.SetFlags(flag)
	ll.warn.SetFlags(flag)
	ll.debug.SetFlags(flag)
}

// StdLogger ...
var (
	StdLogger = NewLogger(log.LstdFlags|log.Lshortfile, 100, 50, 3)
)

// SetJack ...
func SetJack(lfn string, maxsize int) {
	StdLogger.SetJack(lfn, maxsize)
}

// Errorf ...
func Errorf(format string, v ...interface{}) {
	StdLogger.Error(format, v...)
}

// Warnf ...
func Warnf(format string, v ...interface{}) {
	StdLogger.Warn(format, v...)
}

// Infof ...
func Infof(format string, v ...interface{}) {
	StdLogger.Info(format, v...)
}

// Debugf ...
func Debugf(format string, v ...interface{}) {
	StdLogger.Debug(format, v...)
}

// Error ...
func Error(v ...interface{}) {
	StdLogger.Error(GenerateFmtStr(len(v)), v...)
}

// Warn ...
func Warn(v ...interface{}) {
	StdLogger.Warn(GenerateFmtStr(len(v)), v...)
}

// Info ...
func Info(v ...interface{}) {
	StdLogger.Info(GenerateFmtStr(len(v)), v...)
}

// Debug ...
func Debug(v ...interface{}) {
	StdLogger.Debug(GenerateFmtStr(len(v)), v...)
}

// LogLevel ...
func LogLevel(logLevel string) string {
	if len(logLevel) == 0 {
		logLevel = "info"
	}
	updateLevel(logLevel)
	Warn("Set Log Level as", logLevel)
	return logLevel
}

func updateLevel(logLevel string) {
	switch strings.ToLower(logLevel) {
	case "debug":
		StdLogger.SetLevel(LevelDebug)
	case "info":
		StdLogger.SetLevel(LevelInformational)
	case "warn":
		StdLogger.SetLevel(LevelWarning)
	case "error":
		StdLogger.SetLevel(LevelError)
	default:
		StdLogger.SetLevel(LevelInformational)
	}
}

// GenerateFmtStr ...
func GenerateFmtStr(n int) string {
	return strings.Repeat("%v ", n)
}
