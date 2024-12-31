package database

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bpfs/defs/badgerhold"
	"github.com/bpfs/defs/pb"
	"github.com/bpfs/defs/utils/logger"
)

// UploadFileStore 处理 UploadFileRecord 的数据库操作
type UploadFileStore struct {
	db *badgerhold.Store // 数据库连接实例
}

// NewUploadFileStore 创建一个新的 UploadFileStore 实例
// 参数:
//   - db: *badgerhold.Store 数据库连接实例
//
// 返回值:
//   - *UploadFileStore: 新创建的 UploadFileStore 实例
func NewUploadFileStore(db *badgerhold.Store) *UploadFileStore {
	return &UploadFileStore{db: db} // 返回一个新的 UploadFileStore 实例，其中包含传入的数据库实例
}

// CreateUploadFile 创建一个新的上传文件记录
// 参数:
//   - file: *pb.UploadFileRecord 要创建的上传文件记录
//
// 返回值:
//   - error: 如果创建成功返回nil，否则返回错误信息
func (s *UploadFileStore) CreateUploadFile(file *pb.UploadFileRecord) error {
	err := s.db.Upsert(file.TaskId, file) // 将文件记录插入或更新到数据库
	if err != nil {
		logger.Errorf("创建上传文件记录失败: %v", err) // 记录错误日志
		return err
	}
	logger.Infof("成功创建上传文件记录: %s", file.TaskId) // 记录成功日志
	return nil
}

// GetUploadFile 根据任务ID获取上传文件记录
// 参数:
//   - taskID: string 任务的唯一标识符
//
// 返回值:
//   - *pb.UploadFileRecord: 获取到的上传文件记录
//   - bool: 记录是否存在
//   - error: 如果发生系统错误返回错误信息，记录不存在则返回nil
func (s *UploadFileStore) GetUploadFile(taskID string) (*pb.UploadFileRecord, bool, error) {
	var file pb.UploadFileRecord
	err := s.db.Get(taskID, &file)
	if err == badgerhold.ErrNotFound {
		return nil, false, nil // 记录不存在，返回 (nil, false, nil)
	}
	if err != nil {
		logger.Errorf("获取上传文件记录失败: %v", err)
		return nil, false, err // 系统错误，返回 (nil, false, err)
	}
	return &file, true, nil // 记录存在，返回 (record, true, nil)
}

// UpdateUploadFile 更新上传文件记录
// 参数:
//   - file: *pb.UploadFileRecord 要更新的上传文件记录
//
// 返回值:
//   - error: 如果更新成功返回nil，否则返回错误信息
func (s *UploadFileStore) UpdateUploadFile(file *pb.UploadFileRecord) error {
	err := s.db.Update(file.TaskId, file) // 更新数据库中的文件记录
	if err != nil {
		logger.Errorf("更新上传文件记录失败: %v", err) // 记录错误日志
		return err
	}
	logger.Infof("成功更新上传文件记录: %s", file.TaskId) // 记录成功日志
	return nil
}

// UpdateUploadFileStatus 根据任务ID更新文件状态
// 参数:
//   - taskID: string 任务的唯一标识符
//   - status: pb.UploadStatus 要更新的状态
//   - completedAt: int64 完成时间戳
//
// 返回值:
//   - error: 如果更新成功返回nil，否则返回错误信息
func (s *UploadFileStore) UpdateUploadFileStatus(taskID string, status pb.UploadStatus, completedAt int64) error {
	// 获取文件记录
	fileRecord, exists, err := s.GetUploadFile(taskID)
	if err != nil {
		logger.Errorf("获取文件记录失败: taskID=%s, err=%v", taskID, err)
		return err
	}
	if !exists {
		return fmt.Errorf("文件记录不存在: taskID=%s", taskID)
	}

	// 更新状态和完成时间
	fileRecord.Status = status
	fileRecord.FinishedAt = completedAt

	// 更新数据库记录
	err = s.db.Update(taskID, fileRecord)
	if err != nil {
		logger.Errorf("更新文件状态失败: taskID=%s, err=%v", taskID, err)
		return err
	}

	logger.Infof("成功更新文件状态: taskID=%s, status=%v", taskID, status)
	return nil
}

// DeleteUploadFile 删除上传文件记录
// 参数:
//   - taskID: string 要删除的任务ID
//
// 返回值:
//   - error: 如果删除成功返回nil，否则返回错误信息
func (s *UploadFileStore) DeleteUploadFile(taskID string) error {
	err := s.db.Delete(taskID, &pb.UploadFileRecord{}) // 从数据库中删除指定taskID的文件记录
	if err != nil {
		logger.Errorf("删除上传文件记录失败: %v", err) // 记录错误日志
		return err
	}
	logger.Infof("成功删除上传文件记录: %s", taskID) // 记录成功日志
	return nil
}

// ListUploadFiles 列出所有上传文件记录
// 返回值:
//   - []*pb.UploadFileRecord: 所有上传文件记录的切片
//   - error: 如果查询成功返回nil，否则返回错误信息
func (s *UploadFileStore) ListUploadFiles() ([]*pb.UploadFileRecord, error) {
	var files []*pb.UploadFileRecord // 定义切片用于存储所有文件记录
	err := s.db.Find(&files, nil)    // 查询所��文件记录
	if err != nil {
		logger.Errorf("列出所有上传文件记录失败: %v", err) // 记录错误日志
		return nil, err
	}
	return files, nil
}

// GetUploadFilesByStatus 根据状态获取上传文件记录
// 参数:
//   - status: pb.UploadStatus 要查询的上传状态
//
// 返回值:
//   - []*pb.UploadFileRecord: 符合状态条件的文件记录切片
//   - error: 如果查询成功返回nil，否则返回错误信息
func (s *UploadFileStore) GetUploadFilesByStatus(status pb.UploadStatus) ([]*pb.UploadFileRecord, error) {
	var files []*pb.UploadFileRecord                                // 定义切片用于存储查询结果
	err := s.db.Find(&files, badgerhold.Where("Status").Eq(status)) // 查询指定状态的文件记录
	if err != nil {
		logger.Errorf("根据状态获取上传文件记录失败: %v", err) // 记录错误日志
		return nil, err
	}
	return files, nil
}

// GetUploadFilesByFileID 根据文件ID获取上传文件记录
// 参数:
//   - fileID: string 要查询的文件ID
//
// 返回值:
//   - []*pb.UploadFileRecord: 符合文件ID条件的文件记录切片
//   - error: 如果查询成功返回nil，否则返回错误信息
func (s *UploadFileStore) GetUploadFilesByFileID(fileID string) ([]*pb.UploadFileRecord, error) {
	var files []*pb.UploadFileRecord                                // 定义切片用于存储查询结果
	err := s.db.Find(&files, badgerhold.Where("FileId").Eq(fileID)) // 查询指定文件ID的文件记录
	if err != nil {
		logger.Errorf("根据文件ID获取上传文件记录失败: %v", err) // 记录错误日志
		return nil, err
	}
	return files, nil
}

// QueryUploadFiles 高级查询上传文件记录
// 参数:
//   - start: int 分页起始位置
//   - limit: int 每页记录数
//   - param: string 搜索参数
//   - options: ...QueryOption 查询选项
//
// 返回值:
//   - []*pb.UploadFileRecord: 查询结果文件记录切片
//   - uint64: 总记录数
//   - error: 如果查询成功返回nil，否则返回错误信息
func (s *UploadFileStore) QueryUploadFiles(start, limit int, param string, options ...QueryOption) ([]*pb.UploadFileRecord, uint64, error) {
	// 初始化基础查询条件
	query := new(badgerhold.Query)

	if param != "" {
		param = strings.TrimSpace(param)        // 去除参数两端的空白字符
		param = strings.TrimSuffix(param, "\\") // 去除参数末尾的反斜杠
		// 构建正则表达式查询条件
		query = query.And("Path").RegExp(regexp.MustCompile(param)).
			Or(badgerhold.Where("FileId").RegExp(regexp.MustCompile(param))).
			Or(badgerhold.Where("TaskId").RegExp(regexp.MustCompile(param)))
	}

	// 应用查询选项
	for _, option := range options {
		query = option(query)
	}

	var files []*pb.UploadFileRecord                         // 定义切片用于存储查询结果
	err := s.db.Find(&files, query.Skip(start).Limit(limit)) // 执行分页查询
	if err != nil {
		logger.Errorf("查询上传文件记录失败: %v", err) // 记录错误日志
		return nil, 0, err
	}

	totalCount, err := s.db.Count(&pb.UploadFileRecord{}, query) // 获取总记录数
	if err != nil {
		logger.Errorf("获取上传文件记录总数失败: %v", err) // 记录错误日志
		return nil, 0, err
	}

	return files, totalCount, nil
}

// WithStatus 按状态筛选的查询选项
// 参数:
//   - status: pb.UploadStatus 要筛选的上传状态
//
// 返回值:
//   - QueryOption: 返回一个查询选项函数
func WithStatus(status pb.UploadStatus) QueryOption {
	return func(q *badgerhold.Query) *badgerhold.Query {
		return q.And("Status").Eq(status) // 添加状态查询条件
	}
}

// WithTimeRange 按时间范围筛选的查询选项
// 参数:
//   - startTime: int64 开始时间戳
//   - endTime: int64 结束时间戳
//
// 返回值:
//   - QueryOption: 返回一个查询选项函数
func WithUploadTimeRange(startTime, endTime int64) QueryOption {
	return func(q *badgerhold.Query) *badgerhold.Query {
		return q.And("StartedAt").Ge(startTime). // 添加开始时间大于等于条件
								And("StartedAt").Le(endTime) // 添加结束时间小于等于条件
	}
}

// ClearAllUploadFiles 清空所���上传文件记录
// 返回值:
//   - error: 如果清空成功返回nil，否则返回错误信息
func (s *UploadFileStore) ClearAllUploadFiles() error {
	err := s.db.DeleteMatching(&pb.UploadFileRecord{}, nil) // 删除所有文件记录
	if err != nil {
		logger.Errorf("清空所有上传文件记录失败: %v", err) // 记录错误日志
		return err
	}
	logger.Info("所有上传文件记录已成功清空") // 记录成功日志
	return nil
}
