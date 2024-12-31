package downloads

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bpfs/defs/database"
	"github.com/bpfs/defs/reedsolomon"
	"github.com/bpfs/defs/utils/logger"
)

// mergeDownloadedSegments 合并已下载片段
// 返回值:
//   - error: 合并过程中的错误信息
//
// 功能:
//   - 获取并验证所有下载的片段
//   - 使用Reed-Solomon编码合并片段
//   - 生成最终文件
func (t *DownloadTask) mergeDownloadedSegments() error {
	// 创建下载任务存储对象
	downloadFileStore := database.NewDownloadFileStore(t.db)

	// 获取当前任务状态
	fileRecord, exists, err := downloadFileStore.Get(t.taskId)
	if err != nil {
		logger.Errorf("获取任务 %s 状态失败: %v", t.taskId, err)
		return err
	}
	if !exists {
		return fmt.Errorf("任务 %s 不存在", t.taskId)
	}

	// 获取所需的数据片段数量
	requiredShards := getRequiredDataShards(fileRecord.SliceTable)

	// 创建分片文件数组
	shards := make([][]byte, len(fileRecord.SliceTable))
	shardsPresent := 0

	// 获取下载任务所有的切片信息
	segments, err := GetListDownloadSegments(t.db, t.TaskID())
	if err != nil {
		logger.Errorf("获取下载片段列表失败: %v", err)
		return err
	}

	// 读取所有下载的片段
	for index := range fileRecord.SliceTable {
		for _, v := range segments {
			if v.SegmentIndex == index {
				shards[index] = v.SegmentContent
				shardsPresent++
				break
			}
		}
	}

	// 检查是否有足够的分片来重构文件
	if shardsPresent < requiredShards {
		err := fmt.Errorf("没有足够的分片来重构文件，需要 %d 个，但只有 %d 个",
			requiredShards, shardsPresent)
		logger.Errorf(err.Error())
		return err
	}

	// 创建Reed-Solomon编码器
	rs, err := reedsolomon.New(requiredShards, len(shards)-requiredShards)
	if err != nil {
		logger.Errorf("创建Reed-Solomon编码器失败: %v", err)
		return err
	}

	// 创建临时文件用于存储合并后的数据
	tempFilePath := filepath.Join(fileRecord.TempStorage, fileRecord.FileId+".defs")
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		logger.Errorf("创建临时文件失败: %v", err)
		return err
	}
	defer tempFile.Close()

	// 使用JoinFile方法将分片合并为完整文件
	if err := rs.Join(tempFile, shards, int(fileRecord.Size())); err != nil {
		os.Remove(fileRecord.TempStorage)
		logger.Errorf("合并分片失败: %v", err)
		return err
	}

	// 构建基础文件路径
	basePath := filepath.Join(fileRecord.TempStorage, fileRecord.FileMeta.Name)

	// 生成唯一的最终文件路径
	finalFilePath := generateUniqueFilePath(basePath, fileRecord.FileMeta.Extension)

	// 将临时文件重命名为最终文件
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		os.Remove(fileRecord.TempStorage)
		logger.Errorf("重命名文件失败: %v", err)
		return err
	}

	logger.Infof("文件 %s 合并完成，保存在 %s", fileRecord.FileMeta.Name, finalFilePath)

	// 强制触发文件完成处理
	return t.ForceFileFinalize()
}

// generateUniqueFilePath 生成唯一文件路径
// 参数:
//   - basePath: 基础文件路径
//   - extension: 文件扩展名
//
// 返回值:
//   - string: 生成的唯一文件路径
//
// 功能:
//   - 检查文件是否已存在
//   - 如果存在则在文件名后添加递增数字
//   - 生成不冲突的文件路径
func generateUniqueFilePath(basePath, extension string) string {
	// 获取基础路径(不含扩展名)
	basePathWithoutExt := strings.TrimSuffix(basePath, "."+extension)
	finalPath := fmt.Sprintf("%s.%s", basePathWithoutExt, extension)

	// 检查文件是否存在
	counter := 1
	for {
		if _, err := os.Stat(finalPath); os.IsNotExist(err) {
			// 文件不存在，可以使用这个路径
			return finalPath
		}
		// 文件存在，生成新的文件名
		finalPath = fmt.Sprintf("%s_%d.%s", basePathWithoutExt, counter, extension)
		counter++
	}
}
