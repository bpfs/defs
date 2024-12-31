// 定义api错误返回

package api

import (
	"github.com/bpfs/defs/api/pkg/gins"
)

// CODE提示信息
var statusText = map[int]string{
	0: "操作失败",
	1: "操作成功",
}

// 处理响应结果
func HandleResult(code int, result interface{}) gins.ResultObject {
	r := gins.ResultObject{
		Code:    code,
		Message: statusText[code],
		Data:    result,
	}
	return r
}
