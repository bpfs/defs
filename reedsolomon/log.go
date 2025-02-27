package reedsolomon

import logging "github.com/dep2p/log"

var logger = logging.Logger("reedsolomon")

// init 初始化全局日志实例
// 该函数在包初始化时自动执行,用于设置默认的日志配置
func init() {
	// 设置默认的日志配置
	// 使用JSON格式输出,输出到标准错误,日志级别为INFO
	logging.SetupLogging(logging.Config{
		Format: logging.JSONOutput, // 设置输出格式为JSON
		Stderr: true,               // 输出到标准错误
		Level:  logging.LevelInfo,  // 设置日志级别为INFO
	})
}

// SetLog 设置日志配置
// 该方法允许自定义日志输出的文件路径和是否输出到标准错误
// 参数:
// - filename: 日志文件路径,指定日志输出的目标文件
// - stderr: 可选参数,是否同时输出到标准错误,默认为false
func SetLog(filename string, stderr ...bool) {
	// 初始化标准错误输出标志
	useStderr := false
	// 如果提供了stderr参数,则使用提供的值
	if len(stderr) > 0 {
		useStderr = stderr[0]
	}

	// 应用新的日志配置
	// 使用JSON格式输出,可选输出到标准错误,指定日志文件,日志级别为INFO
	logging.SetupLogging(logging.Config{
		Format: logging.JSONOutput, // 设置输出格式为JSON
		Stderr: useStderr,          // 是否输出到标准错误
		File:   filename,           // 设置日志文件路径
		Level:  logging.LevelInfo,  // 设置日志级别为INFO
	})
}
