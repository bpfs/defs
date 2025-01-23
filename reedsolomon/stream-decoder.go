package reedsolomon

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// StreamDecodeFile 从分片文件中解码并恢复原始文件
// 参数:
//   - originalFilePath: 原始文件路径
//   - dataShards: 数据分片数量
//   - parityShards: 校验分片数量
//
// 返回值:
//   - string: 解码后的文件路径
//   - error: 错误信息
func StreamDecodeFile(originalFilePath string, dataShards, parityShards int) (string, error) {
	// 获取原始文件的目录和文件名
	dir, file := filepath.Split(originalFilePath)
	// 构造分片文件所在的目录路径
	outDir := filepath.Join(dir, file+"_shards")
	// 构造分片文件的基础名称
	fname := filepath.Join(outDir, file)
	// 构造解码后的输出文件路径
	outFile := filepath.Join(outDir, "decoded_"+file)

	// 创建编码矩阵
	enc, err := NewStream(dataShards, parityShards)
	if err != nil {
		return "", fmt.Errorf("创建编码矩阵失败: %s", err.Error())
	}

	// 打开输入文件
	shards, size, err := openInput(dataShards, parityShards, fname)
	if err != nil {
		return "", fmt.Errorf("打开输入文件失败: %s", err.Error())
	}

	// 验证分片
	ok, err := enc.Verify(shards)
	if ok {
		// 如果验证通过，打印无需重建的信息
		fmt.Println("无需重建")
	} else {
		// 如果验证失败，打印重建数据的信息
		fmt.Println("验证失败。正在重建数据")
		// 重新打开输入文件
		shards, size, err = openInput(dataShards, parityShards, fname)
		if err != nil {
			return "", fmt.Errorf("重新打开输入文件失败: %s", err.Error())
		}
		// 创建输出目标写入器
		out := make([]io.Writer, len(shards))
		for i := range out {
			// 如果分片为空，创建新的分片文件
			if shards[i] == nil {
				outfn := fmt.Sprintf("%s.%d", fname, i)
				fmt.Printf("正在创建 %s\n", outfn)
				out[i], err = os.Create(outfn)
				if err != nil {
					return "", fmt.Errorf("创建分片文件失败: %s", err.Error())
				}
			}
		}
		// 重建数据
		err = enc.Reconstruct(shards, out)
		if err != nil {
			return "", fmt.Errorf("重建数据失败: %v", err)
		}
		// 关闭输出文件
		for i := range out {
			if out[i] != nil {
				err := out[i].(*os.File).Close()
				if err != nil {
					return "", fmt.Errorf("关闭输出文件失败: %s", err.Error())
				}
			}
		}
		// 重新打开输入文件并验证
		shards, size, err = openInput(dataShards, parityShards, fname)
		ok, err = enc.Verify(shards)
		if !ok {
			return "", fmt.Errorf("重建后验证失败，数据可能已损坏: %v", err)
		}
		if err != nil {
			return "", fmt.Errorf("重建后验证出错: %s", err.Error())
		}
	}

	// 创建输出文件
	fmt.Printf("正在将数据写入 %s\n", outFile)
	f, err := os.Create(outFile)
	if err != nil {
		return "", fmt.Errorf("创建输出文件失败: %s", err.Error())
	}
	defer f.Close()

	// 重新打开输入文件
	shards, size, err = openInput(dataShards, parityShards, fname)
	if err != nil {
		return "", fmt.Errorf("重新打开输入文件失败: %s", err.Error())
	}

	// 合并分片并写入输出文件
	err = enc.Join(f, shards, int64(dataShards)*size)
	if err != nil {
		return "", fmt.Errorf("合并分片并写入输出文件失败: %s", err.Error())
	}

	// 返回解码后的文件路径
	return outFile, nil
}

// openInput 打开输入分片文件
// 参数:
//   - dataShards: 数据分片数量
//   - parityShards: 校验分片数量
//   - fname: 分片文件的基础名称
//
// 返回值:
//   - []io.Reader: 分片文件的读取器切片
//   - int64: 分片文件的大小
//   - error: 错误信息
func openInput(dataShards, parityShards int, fname string) (r []io.Reader, size int64, err error) {
	// 创建分片文件读取器切片
	shards := make([]io.Reader, dataShards+parityShards)
	for i := range shards {
		// 构造分片文件名
		infn := fmt.Sprintf("%s.%d", fname, i)
		fmt.Println("正在打开", infn)
		// 打开分片文件
		f, err := os.Open(infn)
		if err != nil {
			// 如果打开文件失败，打印错误信息并继续下一个文件
			fmt.Println("读取文件时出错", err)
			shards[i] = nil
			continue
		} else {
			// 如果成功打开文件，将文件读取器添加到切片中
			shards[i] = f
		}
		// 获取文件信息
		stat, err := f.Stat()
		if err != nil {
			return nil, 0, fmt.Errorf("获取文件信息失败: %v", err)
		}
		// 记录非空文件的大小
		if stat.Size() > 0 {
			size = stat.Size()
		} else {
			// 如果文件为空，将对应的分片设为nil
			shards[i] = nil
		}
	}
	// 返回分片文件读取器切片、文件大小和错误信息
	return shards, size, nil
}
