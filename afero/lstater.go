package afero

import (
	"os"
)

// Lstater 是 Afero 中的一个可选接口。只有声明支持该接口的文件系统才会实现它。
// 如果文件系统本身是或委托给 os 文件系统，它将调用 Lstat。
// 否则，它将调用 Stat。
// 除了返回 FileInfo，它还会返回一个布尔值，表示是否调用了 Lstat。
type Lstater interface {
	// LstatIfPossible 尽可能调用 Lstat 方法
	// 参数：
	//   - name: string 文件名
	// 返回值：
	//   - os.FileInfo: 文件信息
	//   - bool: 是否调用了 Lstat
	//   - error: 错误信息
	LstatIfPossible(name string) (os.FileInfo, bool, error)
}
