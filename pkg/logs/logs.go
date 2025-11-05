// Package logs 提供了一个简单、灵活且线程安全的日志记录系统。
package logs

import (
	"bytes"
	"fmt"
	"log"
	"sync"
	"time"
)

// LogLevel 表示日志级别的枚举类型
type LogLevel int

// 定义不同的日志级别常量
const (
	None  LogLevel = iota // 无日志输出
	Debug                 // 调试级别
	Info                  // 信息级别
	Warn                  // 警告级别
	Error                 // 错误级别
	Event                 // 事件级别
)

// levelStrings 将日志级别映射到对应的字符串表示
var levelStrings = map[LogLevel]string{
	None:  "NONE",
	Debug: "DEBUG",
	Info:  "INFO",
	Warn:  "WARN",
	Error: "ERROR",
	Event: "EVENT",
}

// ANSI转义序列，用于控制终端输出的颜色
const (
	ansiBlue   = "\033[34m" // 蓝色
	ansiGreen  = "\033[32m" // 绿色
	ansiYellow = "\033[33m" // 黄色
	ansiRed    = "\033[31m" // 红色
	ansiCyan   = "\033[36m" // 青色
	resetColor = "\033[0m"  // 重置颜色
)

// levelColors 将日志级别映射到对应的ANSI颜色代码
var levelColors = map[LogLevel]string{
	None:  "",
	Debug: ansiBlue,
	Info:  ansiGreen,
	Warn:  ansiYellow,
	Error: ansiRed,
	Event: ansiCyan,
}

// Logger 是自定义的日志记录器结构体
type Logger struct {
	mu          sync.Mutex // 互斥锁，确保并发安全
	minLogLevel LogLevel   // 最小日志级别，低于此级别的日志不会被输出
	enableColor bool       // 是否启用彩色输出
	timeFormat  string     // 时间戳格式
}

// logAdapter 是标准日志库的适配器
type logAdapter struct {
	logger *Logger
}

// StdLogger 返回一个标准库日志实例，它将使用当前Logger输出日志
func (l *Logger) StdLogger() *log.Logger {
	return log.New(&logAdapter{logger: l}, "", 0)
}

// Write 实现io.Writer接口，使logAdapter可以作为标准日志库的输出目标
func (a *logAdapter) Write(p []byte) (n int, err error) {
	a.logger.Debug("Internal: %s", string(bytes.TrimSpace(p)))
	return len(p), nil
}

// NewLogger 创建并返回一个新的Logger实例
func NewLogger(logLevel LogLevel, enableColor bool) *Logger {
	if logLevel < None || logLevel > Event {
		logLevel = Info
	}
	return &Logger{
		mu:          sync.Mutex{},
		minLogLevel: logLevel,
		enableColor: enableColor,
		timeFormat:  "2006-01-02 15:04:05.000",
	}
}

// SetLogLevel 设置最小日志级别
func (l *Logger) SetLogLevel(logLevel LogLevel) {
	if l.minLogLevel != logLevel {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.minLogLevel = logLevel
	}
}

// GetLogLevel 获取当前的最小日志级别
func (l *Logger) GetLogLevel() LogLevel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.minLogLevel
}

// EnableColor 设置是否启用彩色输出
func (l *Logger) EnableColor(enable bool) {
	if l.enableColor != enable {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.enableColor = enable
	}
}

// log 是内部日志记录函数，处理通用的日志记录逻辑
func (l *Logger) log(logLevel LogLevel, format string, v ...any) {
	if logLevel < None || logLevel > Event {
		logLevel = Info
	}
	if l.minLogLevel == None {
		return
	}
	if logLevel < l.minLogLevel {
		return
	}

	timestamp := time.Now().Format(l.timeFormat)
	levelStr := levelStrings[logLevel]
	message := fmt.Sprintf(format, v...)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeLog(logLevel, timestamp, levelStr, message)
}

// writeLog 负责实际的日志输出，支持彩色或普通文本输出
func (l *Logger) writeLog(level LogLevel, timestamp, levelStr, message string) {
	if l.enableColor {
		colorCode := levelColors[level]
		fmt.Printf("%s  %s%s%s  %s\n", timestamp, colorCode, levelStr, resetColor, message)
	} else {
		fmt.Printf("%s  %s  %s\n", timestamp, levelStr, message)
	}
}

// Debug 输出调试级别的日志
func (l *Logger) Debug(format string, v ...any) {
	l.log(Debug, format, v...)
}

// Info 输出信息级别的日志
func (l *Logger) Info(format string, v ...any) {
	l.log(Info, format, v...)
}

// Warn 输出警告级别的日志
func (l *Logger) Warn(format string, v ...any) {
	l.log(Warn, format, v...)
}

// Error 输出错误级别的日志
func (l *Logger) Error(format string, v ...any) {
	l.log(Error, format, v...)
}

// Event 输出事件级别的日志
func (l *Logger) Event(format string, v ...any) {
	l.log(Event, format, v...)
}
