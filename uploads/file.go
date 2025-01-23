package uploads

import (
	"crypto/ecdsa"
	"os"
	"time"

	"github.com/bpfs/defs/v2/badgerhold"
	"github.com/bpfs/defs/v2/database"
	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/fscfg"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/shamir"
)

// NewUploadFile 创建并初始化一个新的 UploadFile 实例
// 参数:
//   - opt: *fscfg.Options 存储选项
//   - db: *database.DB 数据库实例
//   - scheme: *shamir.ShamirScheme Shamir 秘密共享方案
//   - name: string 文件名
//   - ownerPriv: *ecdsa.PrivateKey 文件所有者的私钥
//   - onSegmentsReady: func(taskID string) 完成回调函数
//
// 返回值:
//   - *pb.UploadOperationInfo: 上传操作信息
//   - error: 错误信息
func NewUploadFile(opt *fscfg.Options, db *database.DB, scheme *shamir.ShamirScheme,
	name string,
	ownerPriv *ecdsa.PrivateKey,
	onSegmentsReady func(taskID string),
	taskStatus *SegmentStatus,
	errChan chan error,
) (*pb.UploadOperationInfo, error) {
	// 生成任务ID
	taskID, err := files.GenerateTaskID(ownerPriv)
	if err != nil {
		logger.Errorf("生成任务ID失败: %v, name: %s", err, name)
		return nil, err
	}

	// 打开文件
	file, err := os.Open(name)
	if err != nil {
		logger.Errorf("打开文件失败: %v, name: %s", err, name)
		return nil, err
	}
	// 注意，不要在这里关闭，在异步的 NewFileSegment 中关闭

	// 生成 FileMeta 实例
	fileMeta, err := NewFileMeta(file)
	if err != nil {
		logger.Errorf("生成FileMeta失败: %v, name: %s", err, name)
		return nil, err
	}

	// 检查文件大小是否在允许的范围内
	if fileMeta.Size_ < opt.GetMinUploadSize() {
		logger.Errorf("文件大小小于最小上传大小: size=%d, min=%d, name=%s", fileMeta.Size_, opt.GetMinUploadSize(), name)
		return nil, err
	}
	if fileMeta.Size_ > opt.GetMaxUploadSize() {
		logger.Errorf("文件大小大于最大上传大小: size=%d, max=%d, name=%s", fileMeta.Size_, opt.GetMaxUploadSize(), name)
		return nil, err
	}

	// 生成文件ID
	fileID, err := files.GenerateFileID(ownerPriv, fileMeta.Sha256Hash)
	if err != nil {
		logger.Errorf("生成文件ID失败: %v, name: %s", err, name)
		return nil, err
	}

	// 使用文件所有者的私钥和 FileID 生成秘密
	secret, err := files.GenerateSecretFromPrivateKeyAndChecksum(ownerPriv, []byte(fileID))
	if err != nil {
		logger.Errorf("生成秘密失败: %v, fileID: %s", err, fileID)
		return nil, err
	}

	// 创建并初始化 FileSecurity 实例
	fileSecurity, err := NewFileSecurity(fileID, ownerPriv, secret)
	if err != nil {
		logger.Errorf("创建FileSecurity失败: %v, fileID: %s", err, fileID)
		return nil, err
	}

	// 计算分片数量
	dataShards, parityShards, err := CalculateShards(fileMeta.Size_, opt)
	if err != nil {
		logger.Errorf("计算分片数量失败: %v, size: %d", err, fileMeta.Size_)
		return nil, err
	}

	// 计算奇偶校验片段占比
	parityRatio := float64(parityShards) / float64(dataShards+parityShards)

	// 设置初始状态为未指定状态，表示任务状态未初始化或未知
	status := pb.UploadStatus_UPLOAD_STATUS_UNSPECIFIED

	// 创建文件记录并保存到数据库
	if err := CreateUploadFileRecord(
		db.BadgerDB,
		taskID,       // 任务ID
		fileID,       // 文件ID
		name,         // 文件路径
		fileMeta,     // 文件元数据
		fileSecurity, // 文件安全信息
		status,       // 上传状态
	); err != nil {
		logger.Errorf("创建上传文件记录失败: %v, taskID: %s", err, taskID)
		return nil, err
	}

	// 创建上传操作信息
	uploadInfo := &pb.UploadOperationInfo{
		TaskId:   taskID, // 任务ID
		FilePath: name,   // 文件路径

		FileId:       fileID,             // 文件唯一标识
		FileMeta:     fileMeta,           // 文件元数据
		DataShards:   dataShards,         // 数据片段数量
		ParityShards: parityShards,       // 校验片段数量
		SegmentSize:  opt.GetShardSize(), // 文件片段大小
		ParityRatio:  parityRatio,        // 奇偶校验比率
		UploadTime:   time.Now().Unix(),  // 上传时间
		Status:       status,             // 上传状态
	}

	// 异步处理文件分片
	// 启动一个新的 goroutine 来处理文件分片,避免阻塞主流程
	// 只有在前面所有的参数校验和初始化都成功后,才会执行这个异步处理
	go func() {
		// 分片处理完成后,如果设置了回调函数则调用
		// onSegmentsReady 用于通知上层分片准备完成,可以开始上传
		if onSegmentsReady != nil {
			onSegmentsReady(taskID)
		}
		// 调用 NewFileSegment 创建文件分片
		// 参数说明:
		// - db.BadgerDB: 数据库实例,用于存储分片信息
		// - taskID: 任务唯一标识
		// - fileID: 文件唯一标识
		// - file: 文件对象
		// - secret: 加密密钥
		// - dataShards: 数据分片数量
		// - parityShards: 校验分片数量
		if err := NewFileSegment(db.BadgerDB, taskID, fileID, file, secret, dataShards, parityShards); err != nil {
			// 如果分片过程发生错误,记录错误日志
			logger.Errorf("创建文件分片失败: %v", err)
			errChan <- err // 将错误信息传递给外部通道
			return         // 发生错误时直接返回,不会触发 onSegmentsReady 回调
		}

		// 设置任务状态为已就绪
		taskStatus.SetState(true)
	}()

	return uploadInfo, nil
}

// ProcessFileSegments 异步处理文件分片
// 该方法会在后台进行文件的分片、编码和存储操作
//
// 参数:
//   - db: *badgerhold.Store 数据库实例
//   - taskID: string 任务ID
//   - fileID: string 文件ID
//   - file: *os.File 文件对象
//   - pk: []byte 用于加密的公钥
//   - dataShards: int64 数据分片数
//   - parityShards: int64 奇偶校验分片数
//   - onComplete: func() 完成回调函数
func ProcessFileSegments(
	db *badgerhold.Store,
	taskID string,
	fileID string,
	file *os.File,
	pk []byte,
	dataShards,
	parityShards int64,
	onComplete func(),
) {
	go func() {
		defer func() {
			if onComplete != nil {
				onComplete()
			}
		}()

		if err := NewFileSegment(db, taskID, fileID, file, pk, dataShards, parityShards); err != nil {
			logger.Errorf("创建文件分片失败: %v", err)
			return
		}
	}()
}
