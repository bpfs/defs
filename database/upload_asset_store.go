package database

import (
	"regexp"
	"strings"

	"github.com/bpfs/defs/badgerhold"
	"github.com/bpfs/defs/pb"
)

// FileAssetStore 处理 FileAssetRecord 的数据库操作
type FileAssetStore struct {
	db *badgerhold.Store // 数据库连接实例
}

// NewFileAssetStore 创建一个新的 FileAssetStore 实例
// 参数:
//   - db: *badgerhold.Store 数据库连接实例
//
// 返回值:
//   - *FileAssetStore: 新创建的 FileAssetStore 实例
func NewFileAssetStore(db *badgerhold.Store) *FileAssetStore {
	return &FileAssetStore{db: db}
}

// CreateFileAsset 创建一个新的文件资产记录
// 参数:
//   - asset: *pb.FileAssetRecord 要创建的文件资产记录
//
// 返回值:
//   - error: 如果创建成功返回nil，否则返回错误信息
func (s *FileAssetStore) CreateFileAsset(asset *pb.FileAssetRecord) error {
	// 将文件资产记录插入数据库
	err := s.db.Upsert(asset.FileId, asset)
	if err != nil {
		// 如果插入失败，记录错误日志并返回错误
		logger.Errorf("创建文件资产记录失败: %v", err)
		return err
	}
	// 创建成功，返回nil
	return nil
}

// UpdateFileAsset 更新文件资产记录
// 参数:
//   - asset: *pb.FileAssetRecord 要更新的文件资产记录
//
// 返回值:
//   - error: 如果更新成功返回nil，否则返回错误信息
func (s *FileAssetStore) UpdateFileAsset(asset *pb.FileAssetRecord) error {
	// 更新数据库中的文件资产记录
	err := s.db.Update(asset.FileId, asset)
	if err != nil {
		// 如果更新失败，记录错误日志并返回错误
		logger.Errorf("更新文件资产记录失败: %v", err)
		return err
	}
	// 更新成功，返回nil
	return nil
}

// DeleteFileAsset 删除文件资产记录
// 参数:
//   - fileID: string 要删除的文件资产记录的ID
//
// 返回值:
//   - error: 如果删除成功返回nil，否则返回错误信息
func (s *FileAssetStore) DeleteFileAsset(fileID string) error {
	// 从数据库中删除指定ID的文件资产记录
	err := s.db.Delete(fileID, &pb.FileAssetRecord{})
	if err != nil {
		// 如果删除失败，记录错误日志并返回错误
		logger.Errorf("删除文件资产记录失败: %v", err)
		return err
	}
	// 删除成功，返回nil
	return nil
}

// ListAllFileAssets 列出所有文件资产记录
// 返回值:
//   - []*pb.FileAssetRecord: 所有文件资产记录的切片
//   - error: 如果列出成功返回nil，否则返回错误信息
func (s *FileAssetStore) ListAllFileAssets() ([]*pb.FileAssetRecord, error) {
	var assets []*pb.FileAssetRecord
	// 查找所有文件资产记录
	err := s.db.Find(&assets, nil)
	if err != nil {
		logger.Errorf("列出所有文件资产记录失败: %v", err)
		return nil, err
	}
	return assets, nil
}

// QueryFileAssets 查询文件资产记录
// 参数:
//   - pubkeyHash: []byte 所有者的公钥哈希
//   - start: int 查询的起始位置
//   - limit: int 查询的最大记录数
//   - param: string 搜索参数
//   - options: ...QueryOption 可选的查询选项
//
// 返回值:
//   - []*pb.FileAssetRecord: 查询到的文件资产记录切片
//   - uint64: 符合查询条件的总记录数
//   - error: 如果查询成功返回nil，否则返回错误信息
func (s *FileAssetStore) QueryFileAssets(pubkeyHash []byte, start, limit int, param string, options ...QueryOption) ([]*pb.FileAssetRecord, uint64, error) {
	// 创建基本查询条件
	query := badgerhold.Where("PubkeyHash").Eq(pubkeyHash)

	// 添加模糊查询条件
	if param != "" {
		// 去除前后空格和结尾的反斜杠
		param = strings.TrimSpace(param)
		param = strings.TrimSuffix(param, "\\")

		// 添加模糊查询条件
		query = query.And("Name").RegExp(regexp.MustCompile(param)).Or(badgerhold.Where("FileId").RegExp(regexp.MustCompile(param)))
	}

	// 应用所有传入的查询选项
	for _, option := range options {
		query = option(query)
	}

	// 执行查询并获取结果
	var assets []*pb.FileAssetRecord
	err := s.db.Find(&assets, query.Skip(start).Limit(limit))
	if err != nil {
		// 如果查询失败，记录错误日志并返回错误
		logger.Errorf("查询文件资产记录失败: %v", err)
		return nil, 0, err
	}

	// 获取符合条件的总记录数
	totalCount, err := s.db.Count(&pb.FileAssetRecord{}, query)
	if err != nil {
		// 如果获取总数失败，记录错误日志并返回错误
		logger.Errorf("获取文件资产记录总数失败: %v", err)
		return nil, 0, err
	}

	// 返回查询结果、总记录数和nil错误
	return assets, totalCount, nil
}

// QueryOption 定义查询选项函数类型
type QueryOption func(q *badgerhold.Query) *badgerhold.Query

// WithName 按文件名搜索
// 参数:
//   - name: string 要搜索的文件名
//
// 返回值:
//   - QueryOption: 查询选项函数
func WithName(name string) QueryOption {
	return func(q *badgerhold.Query) *badgerhold.Query {
		return q.And("Name").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) {
			// 获取当前记录的文件名
			fileName := ra.Field().(string)
			// 进行不区分大小写的子串匹配
			return strings.Contains(strings.ToLower(fileName), strings.ToLower(name)), nil
		})
	}
}

// WithExtension 按文件扩展名筛选
// 参数:
//   - extension: string 要筛选的文件扩展名
//
// 返回值:
//   - QueryOption: 查询选项函数
func WithExtension(extension string) QueryOption {
	return func(q *badgerhold.Query) *badgerhold.Query {
		// 添加文件扩展名的等值查询条件
		return q.And("Extension").Eq(extension)
	}
}

// WithType 按文件类型筛选（文件或文件夹）
// 参数:
//   - fileType: int32 要筛选的文件类型
//
// 返回值:
//   - QueryOption: 查询选项函数
func WithType(fileType int32) QueryOption {
	return func(q *badgerhold.Query) *badgerhold.Query {
		// 添加文件类型的等值查询条件
		return q.And("Type").Eq(fileType)
	}
}

// WithShared 筛选共享文件
// 参数:
//   - isShared: bool 是否筛选共享文件
//
// 返回值:
//   - QueryOption: 查询选项函数
func WithShared(isShared bool) QueryOption {
	return func(q *badgerhold.Query) *badgerhold.Query {
		// 添加是否共享的等值查询条件
		return q.And("IsShared").Eq(isShared)
	}
}

// WithTimeRange 按上传时间范围筛选
// 参数:
//   - startTime: int64 时间范围的开始时间戳
//   - endTime: int64 时间范围的结束时间戳
//
// 返回值:
//   - QueryOption: 查询选项函数
func WithTimeRange(startTime, endTime int64) QueryOption {
	return func(q *badgerhold.Query) *badgerhold.Query {
		// 添加上传时间范围的查询条件
		return q.And("UploadTime").Ge(startTime).And("UploadTime").Le(endTime)
	}
}

// ClearAllFileAssets 清空所有的文件资产记录
// 返回值:
//   - error: 如果清空成功返回nil，否则返回错误信息
func (s *FileAssetStore) ClearAllFileAssets() error {
	// 删除所有FileAssetRecord类型的记录
	err := s.db.DeleteMatching(&pb.FileAssetRecord{}, nil)
	if err != nil {
		// 如果删除失败，记录错误日志并返回错误
		logger.Errorf("清空所有文件资产记录失败: %v", err)
		return err
	}
	// 清空成功，记录信息日志并返回nil
	logger.Info("所有文件资产记录已成功清空")
	return nil
}
