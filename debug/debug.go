package debug

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// WhereAmI 返回调用它的函数的文件名和行号
// 参数：
//   - depthList: 可选参数，指定调用栈的深度，默认为 1。
//
// 返回值：
//   - string: 调用它的函数的文件名和行号，格式为 "filename:line"。
// func WhereAmI(depthList ...int) string {
// 	var depth int
// 	// 如果 depthList 为空，则将深度设置为 1，否则使用提供的深度值
// 	if depthList == nil {
// 		depth = 1
// 	} else {
// 		depth = depthList[0]
// 	}
// 	// 使用 runtime.Caller 获取调用栈的信息，返回调用者的文件名和行号
// 	_, file, line, _ := runtime.Caller(depth)
// 	// 格式化文件名和行号为 "filename:line"
// 	return fmt.Sprintf("%s:%d", file, line)
// }

// WhereAmI 返回调用它的函数的包名、文件名和行号
// 参数：
//   - depthList: 可选参数，指定调用栈的深度，默认为 1。
//
// 返回值：
//   - string: 调用它的函数的包名、文件名和行号，格式为 "package/file:line"。
func WhereAmI(depthList ...int) string {
	var depth int
	// 如果 depthList 为空，则将深度设置为 1，否则使用提供的深度值
	if depthList == nil {
		depth = 1
	} else {
		depth = depthList[0]
	}
	// 使用 runtime.Caller 获取调用栈的信息，返回调用者的文件名和行号
	pc, file, line, ok := runtime.Caller(depth)
	if !ok {
		return "unknown"
	}
	// 获取函数详细信息
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}
	// 获取完整函数名称
	funcName := fn.Name()
	// 从完整函数名称中提取包名
	parts := strings.Split(funcName, "/")
	pkgFunc := parts[len(parts)-1]
	// 分割包名和函数名
	pkgParts := strings.Split(pkgFunc, ".")
	pkgName := pkgParts[0]

	// 获取文件名
	_, fileName := filepath.Split(file)
	// 格式化包名、文件名和行号为 "package/file:line"
	return fmt.Sprintf("%s/%s:%d", pkgName, fileName, line)
}
