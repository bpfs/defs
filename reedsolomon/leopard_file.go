package reedsolomon

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/utils/logger"
)

// FileEncoder 提供基于文件的Reed-Solomon编解码功能
// 用于处理大文件的流式编码和解码
type FileEncoder interface {
	// EncodeFile 对输入文件数组进行编码,生成校验分片文件
	// 参数:
	//   - shards: 输入文件数组,包含数据分片和校验分片
	// 返回值:
	//   - error: 编码失败时返回相应错误,成功返回nil
	// 说明:
	//   - 分片数量必须与New()函数指定的数量匹配
	//   - 每个分片文件大小必须相同
	//   - 校验分片文件会被覆盖,数据分片文件保持不变
	EncodeFile(shards []*os.File) error

	// VerifyFile 验证文件分片数据的完整性
	// 参数:
	//   - shards: 文件分片数组,包含数据分片和校验分片
	// 返回值:
	//   - bool: 验证通过返回true,否则返回false
	//   - error: 验证过程中出现错误时返回相应错误,成功返回nil
	// 说明:
	//   - 分片数量必须与New()函数指定的数量匹配
	//   - 每个分片件大小必须相同
	//   - 不会修改何数据
	VerifyFile(shards []*os.File) (bool, error)

	// ReconstructFile 重建所有丢失的文件分片
	// 参数:
	//   - shards: 文件分片数组,包含数据分片和校验分片
	// 返回值:
	//   - error: 重建失败时返回相应错误,成功返回nil
	// 说明:
	//   - 分片数量必须等于总分片数
	//   - 通过将分片设置为nil表示丢失的分片
	//   - 如果可用分片太少,将返回ErrTooFewShards错误
	//   - 重建后的分片集合是完整的,但未验证完整性
	ReconstructFile(shards []*os.File) error

	// ReconstructDataFile 仅重建丢失的数据文件分片
	// 参数:
	//   - shards: 文件分片数组,包含数据分片和校验分片
	// 返回值:
	//   - error: 重建失败时返回相应错误,成功返回nil
	// 说明:
	//   - 只重建数据分片,不重建校验分片
	//   - 其他说明同ReconstructFile
	ReconstructDataFile(shards []*os.File) error

	// SplitFile 将输入文件分割成编码器指定数量的临时文件
	// 参数:
	//   - dataFile: 需要分割的输入文件
	// 返回值:
	//   - []*os.File: 分割后的临时文件数组
	//   - error: 分割失败时返回相应错误,成功返回nil
	// 说明:
	//   - 文件将被分割成大小相等的分片
	//   - 如果文件大小不能被分片数整除,最后一个分片将补零
	//   - 使用系统临时目录存储分片文件
	SplitFile(dataFile *os.File) ([]*os.File, error)

	// JoinFile 将文件分片合并到一个输出文件中
	// 参数:
	//   - dst: 输出文件
	//   - shards: 文件分片数组,包含数据分片和校验分片
	//   - outSize: 输出文件的大小
	// 返回值:
	//   - error: 合并失败时返回相应错误,成功返回nil
	// 说明:
	//   - 只考虑数据分片
	//   - 必须提供准确的输出大小
	//   - 如果分片数量不足,将返回ErrTooFewShards错误
	//   - 如果总数据大小小于outSize,将返回ErrShortData错误
	JoinFile(dst *os.File, shards []*os.File, outSize int) error
}

// NewFile 创建一个新的文件编码器并初始化
// 参数:
//   - dataShards: 数据分片数量
//   - parityShards: 校验分片数量
//   - opts: 可选的配置选项
//
// 返回值:
//   - FileEncoder: 创建的文件编码器实例
//   - error: 创建过程中的错误
//
// 说明:
//   - 总分片数量(数据分片+校验分片)最大为65536
//   - 当总分片数大于256时有以下限制:
//   - 分片大小必须是64的倍数
//   - 如果没有提供选项,将使用默认配置
func NewFile(dataShards, parityShards int, opts ...Option) (FileEncoder, error) {
	// 使用默认选项
	o := defaultOptions
	// 应用传入的选项
	for _, opt := range opts {
		opt(&o)
	}

	return newFF16(dataShards, parityShards, o)
}

// 确保leopardFF16实现了FileEncoder接口
var _ = FileEncoder(&leopardFF16{})

// EncodeFile 对输入文件数组进行编码,生成校验分片文件
// 参数:
//   - shards: 输入文件数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 编码失败时返回相应错误,成功返回nil
//
// 说明:
//   - 分片数量必须与New()函数指定的数量匹配
//   - 每个分片文件大小必须相同
//   - 校验分片文件会被覆盖,数据分片文件保持不变
func (r *leopardFF16) EncodeFile(shards []*os.File) error {
	// 检查文件分片参数是否合法
	if err := r.checkFileShards(shards, false); err != nil {
		logger.Error("检查文件分片参数失败:", err)
		return err
	}

	// 调用内部编码方法
	return r.encodeFile(shards)
}

// DecodeFile 从分片文件重建原始文件
// 参数:
//   - outFile: 输出文件,用于存储建的数据
//   - shardDir: 包含分片文件的目录路径
//   - originalSize: 原始文件的大小
//
// 返回值:
//   - error: 解码失败时返回相应错误,成功返回nil
//
// 说明:
//   - 从shardDir目录取分片文件
//   - 使用流式处理避免一次性加载所有分片
//   - 利用对象池复用内存
func (r *leopardFF16) DecodeFile(outFile *os.File, shardDir string, originalSize int64) error {
	if originalSize == 0 {
		logger.Error("原始文件大小为0")
		return ErrShortData
	}

	// 计算每个分片的大小
	perShard := (originalSize + int64(r.dataShards) - 1) / int64(r.dataShards)
	perShard = ((perShard + 63) / 64) * 64

	// 打开所有可用的分片文件
	shardFiles := make([]*os.File, r.totalShards)
	availableShards := 0

	// 打开所有可用的分片文件
	for i := 0; i < r.totalShards; i++ {
		shardPath := filepath.Join(shardDir, fmt.Sprintf("shard_%d.defs", i))
		f, err := os.Open(shardPath)
		if err != nil {
			if !os.IsNotExist(err) {
				logger.Error("打开分片文件失败:", err)
				return err
			}
			continue
		}
		// 验证分片文件大小
		if info, err := f.Stat(); err != nil {
			f.Close()
			logger.Error("获取分片文件信息失败:", err)
			return err
		} else if info.Size()%perShard != 0 {
			f.Close()
			logger.Error("分片文件大小无效")
			return ErrShardSize
		}
		shardFiles[i] = f
		availableShards++
	}
	defer func() {
		for _, f := range shardFiles {
			if f != nil {
				f.Close()
			}
		}
	}()

	// 检查是否有足够的分片用于恢复
	if availableShards < r.dataShards {
		logger.Error("可用分片数量不足")
		return ErrTooFewShards
	}

	// 从对象池获取工作缓冲区
	var shards [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		shards = w
	}
	if cap(shards) >= r.totalShards {
		shards = shards[:r.totalShards]
	} else {
		shards = r.AllocAligned(int(perShard))
	}
	defer r.workPool.Put(&shards)

	// 分块处理文件
	remainingSize := originalSize
	for offset := int64(0); remainingSize > 0; offset += perShard {
		blockSize := perShard
		if remainingSize < blockSize {
			blockSize = remainingSize
		}

		// 读取可用的分片数据
		for i := 0; i < r.totalShards; i++ {
			if shardFiles[i] == nil {
				shards[i] = nil
				continue
			}

			if cap(shards[i]) < int(blockSize) {
				shards[i] = make([]byte, blockSize)
			} else {
				shards[i] = shards[i][:blockSize]
			}

			if _, err := shardFiles[i].ReadAt(shards[i], offset); err != nil {
				logger.Error("读取分片数据失败:", err)
				return err
			}
		}

		// 重建丢失的分片
		if err := r.Reconstruct(shards); err != nil {
			logger.Error("重建分片失败:", err)
			return err
		}

		// 写入重建的数据
		toWrite := blockSize
		if remainingSize < toWrite {
			toWrite = remainingSize
		}

		for i := 0; i < r.dataShards && toWrite > 0; i++ {
			n := toWrite
			if n > int64(len(shards[i])) {
				n = int64(len(shards[i]))
			}
			if _, err := outFile.Write(shards[i][:n]); err != nil {
				logger.Error("写入重建数据失败:", err)
				return err
			}
			toWrite -= n
		}

		remainingSize -= blockSize
	}

	return nil
}

// SplitFile 将输入文件分割成编码器指定数量的临时文件
// 参数:
//   - dataFile: 需要分割的输入文件
//
// 返回值:
//   - []*os.File: 分割后的临时文件数组
//   - error: 分割失败时返回相应错误,成功返回nil
//
// 说明:
// - 文件将被分割成大小相等的分片,每个分片大小必须是64字节的倍数
// - 如果文件大小不能被分片数整除,最后一个分片将包含额外的零值填充
// - 使用系统临时目录存储分片文件
func (r *leopardFF16) SplitFile(dataFile *os.File) ([]*os.File, error) {
	// 获取文件大小
	fileInfo, err := dataFile.Stat()
	if err != nil {
		logger.Error("获取文件信息失败:", err)
		return nil, err
	}
	if fileInfo.Size() == 0 {
		logger.Error("输入文件大小为0")
		return nil, ErrShortData
	}

	// 如果只有一个分片且长度是64的倍数,直接返回原始文件的副本
	if r.totalShards == 1 && fileInfo.Size()&63 == 0 {
		tmpFile, err := os.CreateTemp("", "shard_*.defs")
		if err != nil {
			logger.Error("创建临时文件失败:", err)
			return nil, err
		}
		if _, err := io.Copy(tmpFile, dataFile); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			logger.Error("复制文件内容失败:", err)
			return nil, err
		}
		return []*os.File{tmpFile}, nil
	}

	// 计算每个分片的大小,向上取整到64字节的倍数
	perShard := (fileInfo.Size() + int64(r.dataShards) - 1) / int64(r.dataShards)
	perShard = ((perShard + 63) / 64) * 64

	// 创建临时文件数组
	shardFiles := make([]*os.File, r.totalShards)
	for i := range shardFiles {
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("shard_%d_*.defs", i))
		if err != nil {
			// 清理已创建的临时文件
			for j := 0; j < i; j++ {
				shardFiles[j].Close()
				os.Remove(shardFiles[j].Name())
			}
			logger.Error("创建分片临时文件失败:", err)
			return nil, err
		}
		shardFiles[i] = tmpFile
	}

	// 读取并分割文件
	buffer := make([]byte, perShard)
	remainingSize := fileInfo.Size()
	for i := 0; i < r.dataShards && remainingSize > 0; i++ {
		// 读取一个分片的数据
		n, err := dataFile.Read(buffer)
		if err != nil && err != io.EOF {
			// 清理所有临时文件
			for _, f := range shardFiles {
				f.Close()
				os.Remove(f.Name())
			}
			logger.Error("读取文件数据失败:", err)
			return nil, err
		}

		// 如果读取的数据不足一个完整分片,填充零值
		if n < len(buffer) {
			for j := n; j < len(buffer); j++ {
				buffer[j] = 0
			}
		}

		// 写入分片文件
		if _, err := shardFiles[i].Write(buffer); err != nil {
			// 清理所有临时文件
			for _, f := range shardFiles {
				f.Close()
				os.Remove(f.Name())
			}
			logger.Error("写入分片数据失败:", err)
			return nil, err
		}

		remainingSize -= int64(n)
	}

	// 为剩余的校验分片创建空文件
	for i := r.dataShards; i < r.totalShards; i++ {
		// 写入全零的分片
		zeros := make([]byte, perShard)
		if _, err := shardFiles[i].Write(zeros); err != nil {
			// 清理所有临时文件
			for _, f := range shardFiles {
				f.Close()
				os.Remove(f.Name())
			}
			logger.Error("写入校验分片据失败:", err)
			return nil, err
		}
	}

	// 将所有文件指针重置到开始位置
	for _, f := range shardFiles {
		if _, err := f.Seek(0, 0); err != nil {
			// 清理所有临时文件
			for _, f := range shardFiles {
				f.Close()
				os.Remove(f.Name())
			}
			logger.Error("重置文件指针失败:", err)
			return nil, err
		}
	}

	return shardFiles, nil
}

// encodeFile 执行实际的文件编码操作
// 参数:
//   - shards: 输入文件数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 编码失败时返回相应错误,成功返回nil
func (r *leopardFF16) encodeFile(shards []*os.File) error {
	// 获取分片大小
	shardSize, err := fileShardSize(shards)
	if err != nil {
		return err
	}

	// 重置所有文件指针
	for _, f := range shards {
		if f != nil {
			if _, err := f.Seek(0, 0); err != nil {
				return err
			}
		}
	}

	// 创建工作缓冲区
	data := make([][]byte, r.totalShards)
	for i := range data {
		data[i] = make([]byte, shardSize)
		if i < r.dataShards {
			// 读取数据分片
			if _, err := io.ReadFull(shards[i], data[i]); err != nil {
				return err
			}
		}
	}

	// 编码数据
	if err := r.Encode(data); err != nil {
		return err
	}

	// 写入校验分片
	for i := r.dataShards; i < r.totalShards; i++ {
		if _, err := shards[i].WriteAt(data[i], 0); err != nil {
			return err
		}
	}

	return nil
}

// checkFileShards 检查件分片参数是否合法
// 参数:
//   - shards: 文件分片数组
//   - nilok: 是否允许分片为空
//
// 返回值:
//   - error: 检查失败时返回相应错误,成功返回nil
func (r *leopardFF16) checkFileShards(shards []*os.File, nilok bool) error {
	if len(shards) != r.totalShards {
		logger.Error("分片数量不正确")
		return ErrTooFewShards
	}
	for _, shard := range shards {
		if shard == nil && !nilok {
			logger.Error("存在无效的分片")
			return ErrInvalidInput
		}
	}
	return nil
}

// fileShardSize 获取文件分片的标准大小
// 参数:
//   - shards: 文件分片数组
//
// 返回值:
//   - int: 返回第一个非零文件的大小,如果所有文件都为0则返回0
//   - error: 如果无法获取文件大小,返回相应错误
func fileShardSize(shards []*os.File) (int, error) {
	for _, shard := range shards {
		if shard != nil {
			info, err := shard.Stat()
			if err != nil {
				logger.Error("获取分片文件信息失败:", err)
				return 0, err
			}
			if info.Size() != 0 {
				return int(info.Size()), nil
			}
		}
	}
	logger.Error("所有分片都为空")
	return 0, ErrShardNoData
}

// VerifyFile 验证文件分片数据的完整性
// 参数:
//   - shards: 文件分片数组,包含数据分片和校验分片
//
// 返回值:
//   - bool: 验证通过返回true,否则返回false
//   - error: 验证过程中出现错误时返回相应错误,成功返回nil
//
// 说明:
//   - 通过重新计算校验分片并与原有校验分片比较来验证数据完整性
//   - 如果分片数量不足或分片大小不一致将返回错误
//   - 如果校验分片与重新计算的结果不一致,返回false
func (r *leopardFF16) VerifyFile(shards []*os.File) (bool, error) {
	// 检查分片数量
	if len(shards) != r.totalShards {
		return false, ErrTooFewShards
	}

	// 获取分片大小
	shardSize, err := fileShardSize(shards)
	if err != nil {
		return false, err
	}

	// 重置所有文件指针到开始位置
	for _, f := range shards {
		if f != nil {
			if _, err := f.Seek(0, 0); err != nil {
				return false, err
			}
		}
	}

	// 创建临时缓冲区
	outputs := make([][]byte, r.totalShards)
	for i := range outputs {
		outputs[i] = make([]byte, shardSize)
		if shards[i] != nil {
			// 使用ReadFull确保读取完整数据
			if _, err := io.ReadFull(shards[i], outputs[i]); err != nil {
				return false, err
			}
		}
	}

	// 保存原始校验分片数据
	parityData := make([][]byte, r.parityShards)
	for i := 0; i < r.parityShards; i++ {
		parityData[i] = make([]byte, shardSize)
		copy(parityData[i], outputs[i+r.dataShards])
	}

	// 重新算校验分片
	if err := r.Encode(outputs); err != nil {
		return false, err
	}

	// 比较校验分片
	for i := 0; i < r.parityShards; i++ {
		if !bytes.Equal(outputs[i+r.dataShards], parityData[i]) {
			logger.Printf("校验分片 %d 不一致:\n", i+r.dataShards)
			logger.Printf("期望值: %x\n", parityData[i][:8])
			logger.Printf("实际值: %x\n", outputs[i+r.dataShards][:8])
			return false, nil
		}
	}

	return true, nil
}

// getFileShardSize 获取文件分片的大小
func getFileShardSize(shards []*os.File) (int, error) {
	info, err := shards[0].Stat()
	if err != nil {
		logger.Error("获取分片文件信息失败:", err)
		return 0, err
	}
	return int(info.Size()), nil
}

// ReconstructSomeFile 仅重建指定的文件分片
// 参数:
//   - shards: 文件分片数组,包含数据分片和校验分片
//   - required: 布尔数组,指示需要重建的分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
func (r *leopardFF16) ReconstructSomeFile(shards []*os.File, required []bool) error {
	// 如果required长度等于总分片数,重建所有分片
	if len(required) == r.totalShards {
		return r.reconstructFile(shards, true)
	}
	// 否只重建数据分片
	return r.reconstructFile(shards, false)
}

// ReconstructFile 重建所有丢失的文件分片
// 参数:
//   - shards: 文件分片数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
//
// 说明:
//   - 分片数量必须等于总分片数
//   - 通过将分片设置为nil表示丢失的分片
//   - 如果可用分片太少,将返回ErrTooFewShards错误
//   - 重建后的分片集合是完整的,但未验证完整性
func (r *leopardFF16) ReconstructFile(shards []*os.File) error {
	// 调用内部重建方法,重建所有分片
	return r.reconstructFile(shards, true)
}

// ReconstructDataFile 仅重建丢失的数据文件分片
// 参数:
//   - shards: 文件分片数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
//
// 说明:
//   - 只重建数据分片,不重建校验分片
//   - 其他说明同ReconstructFile
func (r *leopardFF16) ReconstructDataFile(shards []*os.File) error {
	// 调用内部重建方法,只重建数据分片
	return r.reconstructFile(shards, false)
}

// reconstructFile 重建丢失的文件分片数据
// 参数:
//   - shards: 文件分片数组,包含数据分片和校验分片
//   - recoverAll: 是否恢复所有丢失的分片,包括校分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
func (r *leopardFF16) reconstructFile(shards []*os.File, recoverAll bool) error {
	// 检查分片数组长度
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	// 获取分片大小
	shardSize, err := getFileShardSize(shards)
	if err != nil {
		return err
	}

	// 创建临时缓冲区存储所有分片数据
	data := make([][]byte, r.totalShards)
	present := make([]bool, r.totalShards)
	dataPresent := 0

	// 读取现有分片数据
	for i := 0; i < r.totalShards; i++ {
		if shards[i] != nil {
			// 重置文件指针
			if _, err := shards[i].Seek(0, 0); err != nil {
				return err
			}

			// 读取分片数据
			data[i] = make([]byte, shardSize)
			if _, err := io.ReadFull(shards[i], data[i]); err != nil {
				return fmt.Errorf("读取分片 %d 失败: %v", i, err)
			}
			present[i] = true
			dataPresent++
		}
	}

	// 检查是否有足够的分片用于重建
	if dataPresent < r.dataShards {
		return ErrTooFewShards
	}

	// 重建丢失的分片
	if err := r.reconstruct(data, recoverAll); err != nil {
		return err
	}

	// 将重建的数据写入新文件
	for i := 0; i < r.totalShards; i++ {
		if !present[i] {
			// 创建新的临时文件
			tmpFile, err := os.CreateTemp("", fmt.Sprintf("shard_%d_*.defs", i))
			if err != nil {
				return err
			}

			// 写入重建的数据
			if _, err := tmpFile.Write(data[i]); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return err
			}

			// 重置文件指针
			if _, err := tmpFile.Seek(0, 0); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return err
			}

			// 更新分片数组
			shards[i] = tmpFile
		}
	}

	return nil
}

// JoinFile 将文件分片合并到一个输出文件中
// 参数:
//   - dst: 输出文件
//   - shards: 文件分片数组,包含数据分片和校验分片
//   - outSize: 输出文件的大小
//
// 返回值:
//   - error: 合并失败时返回相应错误,成功返回nil
//
// 说明:
//   - 只考虑数据分片
//   - 必须提供准确的输出大小
//   - 如果分片数量不足,将返回ErrTooFewShards错误
//   - 如果总数据大小小于outSize,将返回ErrShortData错误
func (r *leopardFF16) JoinFile(dst *os.File, shards []*os.File, outSize int) error {
	// 检查分片数量
	if len(shards) < r.dataShards {

		return ErrTooFewShards
	}

	// 重置所有文件指针到开始位置
	for _, f := range shards {
		if f != nil {
			if _, err := f.Seek(0, 0); err != nil {
				return fmt.Errorf("重置文件指针失败: %v", err)
			}
		}
	}

	// 获取分片大小
	shardSize, err := fileShardSize(shards)
	if err != nil {
		return err
	}

	// 验证总数据大小
	totalDataSize := int64(shardSize) * int64(r.dataShards)
	if totalDataSize < int64(outSize) {
		return ErrShortData
	}

	// 合并分片到输出文件
	remainingSize := outSize
	buffer := make([]byte, 1024*1024) // 1MB 缓冲区

	for i := 0; i < r.dataShards && remainingSize > 0; i++ {
		if shards[i] == nil {
			return fmt.Errorf("分片 %d 不存在", i)
		}

		// 计算当前分片需要读取的数据量
		toRead := shardSize
		if remainingSize < toRead {
			toRead = remainingSize
		}

		// 分块读写，避免一次性加载过大数据
		bytesRead := 0
		for bytesRead < toRead {
			n := toRead - bytesRead
			if n > len(buffer) {
				n = len(buffer)
			}

			// 读取分片数据
			nr, err := io.ReadFull(shards[i], buffer[:n])
			if err != nil {
				return fmt.Errorf("读取分片 %d 数据失败: %v", i, err)
			}

			// 写入输出文件
			nw, err := dst.Write(buffer[:nr])
			if err != nil {
				return fmt.Errorf("写入输出文件失败: %v", err)
			}
			if nw != nr {
				return fmt.Errorf("写入数据不完整: 期望 %d 字节, 实际写入 %d 字节", nr, nw)
			}

			bytesRead += nr
			remainingSize -= nr
		}
	}

	return nil
}
