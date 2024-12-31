package mem

// Dir 接口定义了目录操作的方法
type Dir interface {
	Len() int           // 返回目录中文件的数量
	Names() []string    // 返回目录中文件的名称
	Files() []*FileData // 返回目录中文件的数据
	Add(*FileData)      // 添加文件到目录
	Remove(*FileData)   // 从目录中移除文件
}

// RemoveFromMemDir 从内存目录中移除文件
// 参数：
//   - dir: *FileData 目录的数据
//   - f: *FileData 要移除的文件数据
func RemoveFromMemDir(dir *FileData, f *FileData) {
	dir.memDir.Remove(f)
}

// AddToMemDir 添加文件到内存目录
// 参数：
//   - dir: *FileData 目录的数据
//   - f: *FileData 要添加的文件数据
func AddToMemDir(dir *FileData, f *FileData) {
	dir.memDir.Add(f)
}

// InitializeDir 初始化目录
// 参数：
//   - d: *FileData 要初始化的目录数据
func InitializeDir(d *FileData) {
	if d.memDir == nil {
		d.dir = true
		d.memDir = &DirMap{}
	}
}
