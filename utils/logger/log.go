package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// Log 是全局日志实例
var Log *logrus.Logger

// DefaultLogger 是默认的日志记录器实现
type DefaultLogger struct {
	*logrus.Logger // 嵌入 logrus.Logger
}

// init 初始化全局日志实例
func init() {
	Log = logrus.New()
	Log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	Log.SetLevel(logrus.InfoLevel)
	Log.SetOutput(os.Stdout)
}

// SetLevel 设置日志级别
// 参数:
//   - level: 日志级别
func SetLevel(level logrus.Level) {
	Log.SetLevel(level)
}

// SetOutput 设置日志输出
// 参数:
//   - output: 输出文件
func SetOutput(output *os.File) {
	Log.SetOutput(output)
}

// whereAmI 返回调用它的函数的包名、文件名和行号
// 参数:
//   - depth: 调用栈的深度
//
// 返回值:
//   - string: 格式化的位置信息
func whereAmI(depth int) string {
	// 获取调用者信息
	pc, file, line, ok := runtime.Caller(depth + 1)
	// 如果���取调用者信息失败，返回"unknown"
	if !ok {
		return "unknown"
	}
	// 获取函数信息
	fn := runtime.FuncForPC(pc)
	// 如果获取函数信息失败，返回"unknown"
	if fn == nil {
		return "unknown"
	}
	// 获取函数名
	funcName := fn.Name()
	// 按"/"分割函数名
	parts := strings.Split(funcName, "/")
	// 获取最后一部分作为包名和函数名
	pkgFunc := parts[len(parts)-1]
	// 按"."分割包名和函数名
	pkgParts := strings.Split(pkgFunc, ".")
	// 获取包名
	pkgName := pkgParts[0]
	// 获取文件名
	_, fileName := filepath.Split(file)
	// 返回格式化的位置信息
	return fmt.Sprintf("[%s/%s:%d]", pkgName, fileName, line)
}

// logWithLocation 使用位置信息记录日志
// 参数:
//   - level: 日志级别
//   - depth: 调用栈深度
//   - args: 日志参数
func logWithLocation(level logrus.Level, depth int, args ...interface{}) {
	// 创建一个带有位置信息的日志条目
	entry := Log.WithField("location", whereAmI(depth+1))
	// 根据日志级别选择相应的日志记录方法
	switch level {
	case logrus.DebugLevel:
		entry.Debug(args...)
	case logrus.InfoLevel:
		entry.Info(args...)
	case logrus.WarnLevel:
		entry.Warn(args...)
	case logrus.ErrorLevel:
		entry.Error(args...)
	case logrus.FatalLevel:
		entry.Fatal(args...)
	case logrus.PanicLevel:
		entry.Panic(args...)
	}
}

// logWithLocationf 使用位置信息记录格式化日志
// 参数:
//   - level: 日志级别
//   - depth: 调用栈深度
//   - format: 格式化字符串
//   - args: 格式化参数
func logWithLocationf(level logrus.Level, depth int, format string, args ...interface{}) {
	// 创建一个带有位置信息的日志条目
	entry := Log.WithField("location", whereAmI(depth+1))
	// 根据日志级别选择相应的格式化日志记录方法
	switch level {
	case logrus.DebugLevel:
		entry.Debugf(format, args...)
	case logrus.InfoLevel:
		entry.Infof(format, args...)
	case logrus.WarnLevel:
		entry.Warnf(format, args...)
	case logrus.ErrorLevel:
		entry.Errorf(format, args...)
	case logrus.FatalLevel:
		entry.Fatalf(format, args...)
	case logrus.PanicLevel:
		entry.Panicf(format, args...)
	}
}

// Debug 记录调试级别的日志
// 参数:
//   - args: 日志参数
func Debug(args ...interface{}) {
	logWithLocation(logrus.DebugLevel, 1, args...)
}

// Debugf 记录格式化的调试级别日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func Debugf(format string, args ...interface{}) {
	logWithLocationf(logrus.DebugLevel, 1, format, args...)
}

// Info 记录信息级别的日志
// 参数:
//   - args: 日志参数
func Info(args ...interface{}) {
	logWithLocation(logrus.InfoLevel, 1, args...)
}

// Infof 记录格式化的信息级别日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func Infof(format string, args ...interface{}) {
	logWithLocationf(logrus.InfoLevel, 1, format, args...)
}

// Warn 记录警告级别的日志
// 参数:
//   - args: 日志参数
func Warn(args ...interface{}) {
	logWithLocation(logrus.WarnLevel, 1, args...)
}

// Warnf 记录格式化的警告级别日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func Warnf(format string, args ...interface{}) {
	logWithLocationf(logrus.WarnLevel, 1, format, args...)
}

// Error 记录错误级别的日志
// 参数:
//   - args: 日志参数
func Error(args ...interface{}) {
	logWithLocation(logrus.ErrorLevel, 1, args...)
}

// Errorf 记录格式化的错误级别日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func Errorf(format string, args ...interface{}) {
	logWithLocationf(logrus.ErrorLevel, 1, format, args...)
}

// Fatal 记录致命级别的日志
// 参数:
//   - args: 日志参数
func Fatal(args ...interface{}) {
	logWithLocation(logrus.FatalLevel, 1, args...)
}

// Fatalf 记录格式化的致命级别日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func Fatalf(format string, args ...interface{}) {
	logWithLocationf(logrus.FatalLevel, 1, format, args...)
}

// Panic 记录 panic 级别的日志
// 参数:
//   - args: 日志参数
func Panic(args ...interface{}) {
	logWithLocation(logrus.PanicLevel, 1, args...)
}

// Panicf 记录格式化的 panic 级别日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func Panicf(format string, args ...interface{}) {
	logWithLocationf(logrus.PanicLevel, 1, format, args...)
}

// WithFields 创建一个带有字段的日志条目
// 参数:
//   - fields: 日志字段
//
// 返回值:
//   - *logrus.Entry: 带有字段的日志条目
func WithFields(fields logrus.Fields) *logrus.Entry {
	return Log.WithFields(fields)
}

// WithField 创建一个带有单个字段的日志条目
// 参数:
//   - key: 字段键
//   - value: 字段值
//
// 返回值:
//   - *logrus.Entry: 带有单个字段的日志条目
func WithField(key string, value interface{}) *logrus.Entry {
	return Log.WithField(key, value)
}

// Print 记录信息级别的日志
// 参数:
//   - args: 日志参数
func Print(args ...interface{}) {
	logWithLocation(logrus.InfoLevel, 1, args...)
}

// Printf 记录格式化的信息级别日志
// 参数:
//   - format: 格式化字符串
//   - args: 格式化参数
func Printf(format string, args ...interface{}) {
	logWithLocationf(logrus.InfoLevel, 1, format, args...)
}

// Println 记录信息级别的日志，并在末尾添加换行符
// 参数:
//   - args: 日志参数
func Println(args ...interface{}) {
	logWithLocation(logrus.InfoLevel, 1, append(args, "\n")...)
}

// Trace 记录跟踪级别的日志
func Trace(args ...interface{}) {
	logWithLocation(logrus.TraceLevel, 1, args...)
}

// Tracef 记录格式化的跟踪级别日志
func Tracef(format string, args ...interface{}) {
	logWithLocationf(logrus.TraceLevel, 1, format, args...)
}

// LogMessage 记录指定级别的日志
func LogMessage(level logrus.Level, args ...interface{}) {
	logWithLocation(level, 1, args...)
}

// Logf 记录格式化的指定级别日志
func Logf(level logrus.Level, format string, args ...interface{}) {
	logWithLocationf(level, 1, format, args...)
}

// WithError 创建一个包含错误信息的日志条目
func WithError(err error) *logrus.Entry {
	return Log.WithError(err)
}

// SetReportCaller 设置是否在日志中报告调用者信息
func SetReportCaller(reportCaller bool) {
	Log.SetReportCaller(reportCaller)
}

// ParseLevel 解析字符串表示���日志级别
func ParseLevel(level string) (logrus.Level, error) {
	return logrus.ParseLevel(level)
}

// GetLevel 获取当前的日志级别
func GetLevel() logrus.Level {
	return Log.GetLevel()
}

// SetStdoutEnabled 设置是否启用标准输出
func SetStdoutEnabled(enabled bool) {
	if enabled {
		// 同时输出到标准输出和文件
		if Log.Out != os.Stdout {
			Log.SetOutput(io.MultiWriter(os.Stdout, Log.Out))
		}
	} else {
		// 如果当前输出包含标准输出，则只保留文件输出
		if Log.Out != nil && Log.Out != os.Stdout {
			Log.SetOutput(Log.Out)
		}
	}
}
