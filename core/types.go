package core

import "time"

// FileInfo 描述一个文件
type FileInfo struct {
	fileID      string            // 文件的唯一标识
	fileKey     string            // 文件的密钥
	name        string            // 文件的名称
	size        int64             // 文件的长度(以字节为单位)
	uploadTime  time.Time         // 上传时间
	modTime     time.Time         // 修改时间(非文件修改时间)
	fileType    string            // 文件类型或格式
	p2pkhScript []byte            // P2PKH 脚本(用于所有者)
	p2pkScript  []byte            // P2PK 脚本(用于验签)
	sliceTable  map[int]HashTable // 文件片段的哈希表
	sliceList   []SliceInfo       // 文件片段的列表
}

// HashTable 描述了哈希表的内容
type HashTable struct {
	Hash      string // 文件片段的哈希值
	IsRsCodes bool   // 是否为纠删码
}

// SliceInfo 描述了文件的一个文件片段信息
type SliceInfo struct {
	index     int    // 文件片段的索引(该片段在文件中的顺序位置)
	sliceHash string // 文件片段的哈希值(外部标识)
	/**
	数据签名字段：
		fileID			文件的唯一标识(外部标识)
		sliceTable		文件片段内容的哈希表
		sliceHash		文件片段的哈希值(外部标识)
		index			文件片段的索引(该片段在文件中的顺序位置)
		mode			文件片段的存储模式
	写入 P2PK 字段。
	远程节点使用 P2PK 进行验签；本地下载使用 fileKey 进行验签。
	*/
	signature []byte // 文件和文件片段的数据签名
}

// UploadChan 用于刷新上传的通道
type UploadChan struct {
	FileID      string   // 文件的唯一标识(外部标识)
	SliceHash   string   // 文件片段的哈希值(外部标识)
	TotalPieces int      // 文件总片数
	Index       int      // 文件片段的索引(该片段在文件中的顺序位置)
	Pid         []string // 节点ID
}

// DownloadChan 用于刷新下载的通道
type DownloadChan struct {
	FileID      string // 文件的唯一标识(外部标识)
	SliceHash   string // 文件片段的哈希值(外部标识)
	TotalPieces int    // 文件总片数（数据片段和纠删码片段的总数）
	Index       int    // 文件片段的索引(该片段在文件中的顺序位置)
}

// GetFileID 获取文件的唯一标识
func (fi *FileInfo) GetFileID() string {
	return fi.fileID
}

// BuildFileID 设置文件的唯一标识
func (fi *FileInfo) BuildFileID(fileID string) {
	fi.fileID = fileID
}

// GetFileKey 获取文件的密钥
func (fi *FileInfo) GetFileKey() string {
	return fi.fileKey
}

// BuildFileKey 设置文件的哈希值
func (fi *FileInfo) BuildFileKey(fileKey string) {
	fi.fileKey = fileKey
}

// GetName 获取文件的基本名称
func (fi *FileInfo) GetName() string {
	return fi.name
}

// Name 设置文件的基本名称
func (fi *FileInfo) BuildName(name string) {
	fi.name = name
}

// GetSize 获取文件的长度（以字节为单位）
func (fi *FileInfo) GetSize() int64 {
	return fi.size
}

// Size 设置文件的长度（以字节为单位）
func (fi *FileInfo) BuildSize(size int64) {
	fi.size = size
}

// GetUploadTime 获取文件的上传时间
func (fi *FileInfo) GetUploadTime() time.Time {
	return fi.uploadTime
}

// UploadTime 设置上传时间
func (fi *FileInfo) BuildUploadTime(uploadTime time.Time) {
	fi.uploadTime = uploadTime
}

// GetModTime 获取文件的修改时间
func (fi *FileInfo) GetModTime() time.Time {
	return fi.modTime
}

// BuildModTime 设置修改时间
func (fi *FileInfo) BuildModTime(modTime time.Time) {
	fi.modTime = modTime
}

// GetFileType 获取文件的类型或格式
func (fi *FileInfo) GetFileType() string {
	return fi.fileType
}

// BuildFileType 设置文件类型或格式
func (fi *FileInfo) BuildFileType(fileType string) {
	fi.fileType = fileType
}

// GetP2pkhScript 获取文件的 P2PKH 脚本
func (fi *FileInfo) GetP2pkhScript() []byte {
	return fi.p2pkhScript
}

// BuildP2pkhScript 设置文件的 P2PKH 脚本
func (fi *FileInfo) BuildP2pkhScript(p2pkhScript []byte) {
	fi.p2pkhScript = p2pkhScript
}

// GetP2pkScript 获取文件的 P2PK 脚本
func (fi *FileInfo) GetP2pkScript() []byte {
	return fi.p2pkScript
}

// BuildP2pkScript 设置文件的 P2PK 脚本
func (fi *FileInfo) BuildP2pkScript(p2pkScript []byte) {
	fi.p2pkScript = p2pkScript
}

// GetSliceTable 获取文件片段的哈希表
func (fi *FileInfo) GetSliceTable() map[int]HashTable {
	return fi.sliceTable
}

// BuildSliceTable 设置哈希表
func (fi *FileInfo) BuildSliceTable() {
	fi.sliceTable = make(map[int]HashTable)
}

// AddSliceTable 向哈希表添加新的文件片段内容
func (fi *FileInfo) AddSliceTable(index int, hash string, rc bool) {
	hashTable := HashTable{
		Hash:      hash,
		IsRsCodes: rc,
	}
	fi.sliceTable[index] = hashTable
}

// DelSliceTable 删除哈希表中的文件片段内容
func (fi *FileInfo) DelSliceTable(k int) {
	delete(fi.sliceTable, k)
}

// GetSliceList 获取文件片段的列表
func (fi *FileInfo) GetSliceList() []SliceInfo {
	return fi.sliceList
}

// BuildSliceList 设置文件片段的列表
func (fi *FileInfo) BuildSliceList(len int) {
	fi.sliceList = make([]SliceInfo, 0, len) // 提前设定大小
}

// AddSliceList 向列表添加新的文件片段内容
func (fi *FileInfo) AddSliceList(sliceInfo *SliceInfo) {
	fi.sliceList = append(fi.sliceList, *sliceInfo)
}

// DelSliceList 删除列表中的文件片段内容
func (fi *FileInfo) DelSliceList(k int) {

}

// BuildSliceInfo 设置一个文件片段的信息
func BuildSliceInfo(index int, sliceHash string, signature []byte) *SliceInfo {
	return &SliceInfo{
		index:     index,
		sliceHash: sliceHash,
		signature: signature,
	}
}

// GetIndex 获取文件片段的索引
func (si *SliceInfo) GetIndex() int {
	return si.index
}

// GetSliceHash 获取文件片段的哈希值
func (si *SliceInfo) GetSliceHash() string {
	return si.sliceHash
}

// GetSignature 获取文件和文件片段的数据签名
func (si *SliceInfo) GetSignature() []byte {
	return si.signature
}
