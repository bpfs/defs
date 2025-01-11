package files

import (
	"github.com/bpfs/defs/v2/afero"
	"github.com/bpfs/defs/v2/utils/paths"
	"go.uber.org/fx"
)

// NewAferoFsInput 是用于传递给 NewAferoFs 函数的输入结构体。
type NewAferoFsInput struct {
	fx.In
}

// NewAferoFsOutput 是 NewAferoFs 函数的输出结构体。
type NewAferoFsOutput struct {
	fx.Out

	Fs afero.Afero
}

// NewAferoFs 创建并返回一个新的 afero.Afero 实例
// 参数:
//   - lc: fx.Lifecycle 对象，用于管理生命周期
//   - input: NewAferoFsInput 结构体，包含输入参数
//
// 返回值:
//   - out: NewAferoFsOutput 结构体，包含输出结果
//   - err: 错误信息，如果没有错误则为 nil
func NewAferoFs(lc fx.Lifecycle, input NewAferoFsInput) (out NewAferoFsOutput, err error) {
	// 获取根路径
	rootPath := paths.GetRootPath()

	// 创建一个新的 OsFs 实例
	// OsFs 是对操作系统文件系统的封装
	fs := afero.NewOsFs()

	// 创建一个新的 BasePathFs，使用根路径作为基础路径
	// BasePathFs 将所有操作限制在指定的基础路径内
	baseFs := afero.NewBasePathFs(fs, rootPath)

	// 将创建的文件系统赋值给输出结构体
	out.Fs = baseFs

	// 返回输出结构体和 nil 错误
	return out, nil
}
