package log

import logging "github.com/dep2p/log"

// init 初始化全局日志实例
func init() {
	// 设置JSON格式以便于解析
	logging.SetupLogging(logging.Config{
		Format: logging.JSONOutput,
		Stderr: true,
		Level:  logging.LevelInfo,
	})
}
func SetLog(filename string, stderr ...bool) {
	useStderr := false
	if len(stderr) > 0 {
		useStderr = stderr[0]
	}
	// 设置JSON格式以便于解析
	logging.SetupLogging(logging.Config{
		Format: logging.JSONOutput,
		Stderr: useStderr,
		File:   filename,
		Level:  logging.LevelInfo,
	})
}
