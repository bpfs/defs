//go:build ignore
// +build ignore

// Copyright 2023, Your Name, see LICENSE for details.

// 大文件流解码器示例

// 这个实现的主要特点：
// 1. 它能够处理被分批编码的大文件，每个批次都独立解码。
// 2. 它会自动检测并处理所有可用的批次。
// 3. 它支持数据重建，如果某些分片丢失或损坏。
// 4. 解码后的数据会被追加到同一个输出文件中，重建原始文件。
// 要使用这个解码器，您可以这样编译和运行：
//
// go build large-file-decoder.go
// ./large-file-decoder -data 17 -par 3 大文件.bin.batch0.shard0
//
// 这将从 大文件.bin.batch0.shard0 开始，处理所有批次，并重建原始的 大文件.bin。
// 请注意，这个解码器假设所有批次都使用相同的数据分片和奇偶校验分片数量。
// 如果在实际应用中这些参数可能会变化，您可能需要在每个批次的元数据中存储这些信息。

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bpfs/defs/reedsolomon"
	"github.com/bpfs/defs/utils/logger"
)

// 定义全局变量
var (
	// 定义命令行参数：数据分片数量
	dataShards = flag.Int("data", 17, "每批数据分片数量")
	// 定义命令行参数：奇偶校验分片数量
	parShards = flag.Int("par", 3, "每批奇偶校验分片数量")
	// 定义命令行参数：输出文件路径
	outFile = flag.String("out", "", "输出文件路径")
	// 定义Reed-Solomon解码的最大批次大小
	maxBatchSize = 256 // Reed-Solomon 解码的最大批次大小
)

// init 函数：初始化设置
func init() {
	// 设置命令行使用说明
	flag.Usage = func() {
		// 打印使用方法
		fmt.Fprintf(os.Stderr, "使用方法: %s [-flags] filename.ext.batch0.shard0\n\n", os.Args[0])
		// 打印有效的标志说明
		fmt.Fprintf(os.Stderr, "有效的标志:\n")
		// 打印默认标志
		flag.PrintDefaults()
	}
}

// main 函数：程序入口点
func main() {
	// 解析命令行参数
	flag.Parse()
	// 检查是否提供了输入文件名
	if flag.NArg() != 1 {
		logger.Error("错误: 未提供输入文件名")
		flag.Usage()
		os.Exit(1)
	}

	// 检查分片数量是否超过最大批次大小
	if *dataShards+*parShards > maxBatchSize {
		logger.Errorf("错误: 数据分片和奇偶校验分片的总和不能超过 %d", maxBatchSize)
		os.Exit(1)
	}

	// 获取输入文件名
	inputFile := flag.Arg(0)
	logger.Infof("正在处理 %s", inputFile)

	// 解析文件名以获取基本文件名和批次信息
	baseName, batchNum, err := parseFileName(inputFile)
	if err != nil {
		logger.Errorf("解析文件名失败: %v", err)
		os.Exit(1)
	}

	// 创建Reed-Solomon解码器
	enc, err := reedsolomon.NewStream(*dataShards, *parShards)
	if err != nil {
		logger.Errorf("创建Reed-Solomon解码器失败: %v", err)
		os.Exit(1)
	}

	// 设置输出文件名
	outName := *outFile
	if outName == "" {
		outName = baseName
	}

	// 创建输出文件
	f, err := os.Create(outName)
	if err != nil {
		logger.Errorf("创建输出文件 %s 失败: %v", outName, err)
		os.Exit(1)
	}
	defer f.Close()

	logger.Infof("正在创建输出文件: %s", outName)

	// 初始化总写入字节数
	totalBytesWritten := int64(0)

	// 处理所有批次
	for {
		// 处理当前批次
		bytesWritten, err := processBatch(enc, baseName, batchNum, f)

		// 检查是否所有批次处理完毕
		if err == io.EOF {
			logger.Info("所有批次处理完毕")
			break
		}
		// 检查处理批次是否出错
		if err != nil {
			logger.Errorf("处理批次 %d 失败: %v", batchNum, err)
			os.Exit(1)
		}
		// 累加写入的字节数
		totalBytesWritten += bytesWritten
		// 增加批次号
		batchNum++
	}

	// 获取最终文件大小
	finalSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		logger.Errorf("获取最终文件大小失败: %v", err)
		os.Exit(1)
	}

	// 打印解码完成信息
	logger.Infof("解码完成。输出文件: %s", outName)
	logger.Infof("总共写入 %d 字节", totalBytesWritten)
	logger.Infof("最终文件大小: %d 字节", finalSize)

	// 检查写入的字节数是否与最终文件大小匹配
	if finalSize != totalBytesWritten {
		logger.Warnf("警告：写入的字节数 (%d) 与最终文件大小 (%d) 不匹配", totalBytesWritten, finalSize)
	}

	// 检查输出文件是否为空
	if finalSize == 0 {
		logger.Error("错误：输出文件是空的")
	}
}

// parseFileName 函数：解析文件名以获取基本文件名和批次号
// 参数：
//   - fileName: 输入文件名
//
// 返回值：
//   - string: 基本文件名
//   - int: 批次号
//   - error: 错误信息
func parseFileName(fileName string) (string, int, error) {
	// 记录解析文件名的日志
	logger.Infof("解析文件名: %s", fileName)
	// 获取文件所在目录
	dir := filepath.Dir(fileName)
	// 获取文件基本名称
	base := filepath.Base(fileName)
	// 记录目录和基本文件名的日志
	logger.Infof("目录: %s, 基本文件: %s", dir, base)
	// 按点分割文件名
	parts := strings.Split(base, ".")
	// 记录文件名各部分的日志
	logger.Infof("文件名部分: %v", parts)
	// 检查文件名格式是否有效
	if len(parts) < 4 {
		return "", 0, fmt.Errorf("无效的文件名格式")
	}
	// 保留原始文件名（包括所有扩展名）
	baseName := strings.Join(parts[:len(parts)-2], ".")
	// 初始化批次号变量
	var batchNum int
	// 从文件名中解析批次号
	_, err := fmt.Sscanf(parts[len(parts)-2], "batch%d", &batchNum)
	if err != nil {
		logger.Errorf("解析批次号失败: %v", err)
	}
	// 记录解析结果的日志
	logger.Infof("解析结果 - 基本文件名: %s, 批次号: %d", baseName, batchNum)
	// 返回完整的基本文件名（包括路径）、批次号和可能的错误
	return filepath.Join(dir, baseName), batchNum, err
}

// processBatch 函数：处理单个批次的解码
// 参数：
//   - enc: Reed-Solomon编码器
//   - baseName: 基本文件名
//   - batchNum: 批次号
//   - outFile: 输出文件
//
// 返回值：
//   - int64: 写入的字节数
//   - error: 错误信息
func processBatch(enc reedsolomon.StreamEncoder, baseName string, batchNum int, outFile *os.File) (int64, error) {
	// 记录正在处理的批次号
	logger.Infof("正在处理批次 %d", batchNum)

	// 打开输入分片文件
	shards, closers, size, err := openInputShards(baseName, batchNum, *dataShards, *parShards)
	if err != nil {
		logger.Errorf("打开输入分片失败: %v", err)
		return 0, io.EOF
	}
	// 确保在函数结束时关闭所有打开的文件
	defer func() {
		for _, closer := range closers {
			if closer != nil {
				closer.Close()
			}
		}
	}()

	// 验证分片
	ok, err := enc.Verify(shards)
	if err != nil {
		logger.Errorf("验证分片失败: %v", err)
		return 0, err
	}

	// 如果验证失败，尝试重建数据
	if !ok {
		logger.Warn("验证失败。正在尝试重建数据。")
		err = enc.Reconstruct(shards, nil)
		if err != nil {
			logger.Errorf("重建数据失败: %v", err)
			return 0, err
		}

		// 重建后再次验证
		ok, err = enc.Verify(shards)
		if err != nil {
			logger.Errorf("重建后验证失败: %v", err)
			return 0, err
		}
		if !ok {
			logger.Error("重建后验证仍然失败，数据可能已损坏")
			return 0, fmt.Errorf("重建后验证仍然失败，数据可能已损坏")
		}
		logger.Info("数据重建成功")
	}

	// 记录写入前的文件位置
	startPos, err := outFile.Seek(0, io.SeekCurrent)
	if err != nil {
		logger.Errorf("获取文件当前位置失败: %v", err)
		return 0, err
	}

	// 计算实际的数据大小
	dataSize := int64(0)
	for i := 0; i < *dataShards; i++ {
		if shards[i] != nil {
			if reader, ok := shards[i].(*os.File); ok {
				fi, err := reader.Stat()
				if err != nil {
					logger.Errorf("获取分片 %d 文件信息失败: %v", i, err)
					return 0, fmt.Errorf("获取分片文件信息失败: %v", err)
				}
				logger.Infof("分片 %d 大小: %d 字节", i, fi.Size())
				dataSize += fi.Size()
			} else {
				logger.Errorf("分片 %d 不是文件类型", i)
				return 0, fmt.Errorf("无法获取分片 %d 的大小", i)
			}
		} else {
			logger.Warnf("分片 %d 为空", i)
		}
	}

	// 记录批次的实际数据大小
	logger.Infof("批次 %d: 实际数据大小 %d 字节", batchNum, dataSize)
	logger.Infof("Join 参数: outFile=%v, shards长度=%d, dataSize=%d", outFile.Name(), len(shards), dataSize)

	// 重新打开输入分片文件
	shards, closers, size, err = openInputShards(baseName, batchNum, *dataShards, *parShards)
	if err != nil {
		logger.Errorf("打开输入分片失败: %v", err)
		return 0, err
	}

	// 合并分片并写入输出文件
	err = enc.Join(outFile, shards, int64(*dataShards)*size)
	if err != nil {
		logger.Errorf("合并分片失败: %v", err)
		return 0, err
	}

	// 记录写入后的文件位置
	endPos, err := outFile.Seek(0, io.SeekCurrent)
	if err != nil {
		logger.Errorf("获取文件结束位置失败: %v", err)
		return 0, err
	}

	// 计算写入的字节数
	bytesWritten := endPos - startPos
	logger.Infof("批次 %d 已成功解码，写入 %d 字节", batchNum, bytesWritten)

	// 检查写入的字节数是否与预期相符
	if bytesWritten != dataSize {
		logger.Warnf("写入的字节数与预期不符，差异: %d 字节", dataSize-bytesWritten)
	}

	return bytesWritten, nil
}

// openInputShards 函数：打开输入分片文件
// 参数：
//   - baseName: 基本文件名
//   - batchNum: 批次号
//   - dataShards: 数据分片数量
//   - parShards: 奇偶校验分片数量
//
// 返回值：
//   - []io.Reader: 分片读取器数组
//   - []io.Closer: 分片关闭器数组
//   - int64: 分片大小
//   - error: 错误信息
func openInputShards(baseName string, batchNum, dataShards, parShards int) ([]io.Reader, []io.Closer, int64, error) {
	// 创建分片读取器和关闭器数组
	shards := make([]io.Reader, dataShards+parShards)
	closers := make([]io.Closer, dataShards+parShards)
	var size int64
	var nonEmptyShards int

	// 遍历所有分片
	for i := range shards {
		// 构造分片文件名
		fileName := fmt.Sprintf("%s.batch%d.shard%d", baseName, batchNum, i)
		logger.Infof("尝试打开文件: %s", fileName)
		// 打开分片文件
		f, err := os.Open(fileName)
		if err != nil {
			if os.IsNotExist(err) {
				logger.Warnf("文件不存在: %s", fileName)
				if i < dataShards {
					logger.Errorf("数据分片 %d 不存在，这可能导致解码失败", i)
				}
				continue
			}
			logger.Errorf("无法打开 %s: %v", fileName, err)
			continue
		}

		// 获取文件信息
		stat, err := f.Stat()
		if err != nil {
			f.Close()
			logger.Errorf("获取文件信息失败: %v", err)
			continue
		}
		// 检查文件是否为空
		if stat.Size() > 0 {
			size = stat.Size()
			shards[i] = f
			closers[i] = f
			nonEmptyShards++
			logger.Infof("成功打开分片文件 %s，大小: %d 字节", fileName, stat.Size())
		} else {
			f.Close()
			logger.Warnf("分片文件 %s 是空的", fileName)
		}
	}

	// 记录找到的非空分片数量和大小
	logger.Infof("找到 %d 个非空分片，每个分片大小 %d 字节", nonEmptyShards, size)
	logger.Infof("预期分片数: %d (数据: %d, 奇偶校验: %d)", dataShards+parShards, dataShards, parShards)

	// 检查是否所有分片都是空的或不存在
	if nonEmptyShards == 0 {
		return nil, nil, 0, fmt.Errorf("所有分片文件都是空的或不存在")
	}

	return shards, closers, size, nil
}
