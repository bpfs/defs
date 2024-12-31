package mem

import "sort"

// DirMap 是一个 map，键为字符串，值为 *FileData
type DirMap map[string]*FileData

// Len 返回目录中文件的数量
func (m DirMap) Len() int {
	return len(m)
}

// Add 将文件添加到目录
// 参数：
//   - f: *FileData 要添加的文件数据
func (m DirMap) Add(f *FileData) {
	m[f.name] = f
}

// Remove 从目录中移除文件
// 参数：
//   - f: *FileData 要移除的文件数据
func (m DirMap) Remove(f *FileData) {
	delete(m, f.name)
}

// Files 返回目录中文件的数据，按文件名排序
func (m DirMap) Files() (files []*FileData) {
	for _, f := range m {
		files = append(files, f)
	}
	sort.Sort(filesSorter(files))
	return files
}

// filesSorter 实现了 sort.Interface，用于对 []*FileData 进行排序
type filesSorter []*FileData

// Len 返回文件数组的长度
func (s filesSorter) Len() int {
	return len(s)
}

// Swap 交换文件数组中两个元素的位置
func (s filesSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less 比较文件数组中两个元素的大小，按文件名排序
func (s filesSorter) Less(i, j int) bool {
	return s[i].name < s[j].name
}

// Names 返回目录中文件的名称
func (m DirMap) Names() (names []string) {
	for x := range m {
		names = append(names, x)
	}
	return names
}
