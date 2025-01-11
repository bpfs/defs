//go:build ignore
// +build ignore

// Copyright 2023, Your Name, see LICENSE for details.

// 大文件流编码器示例
//
// 这个实现的主要特点：
// 1. 它将大文件分成多个批次处理，每个批次都符合Reed-Solomon编码的256个分片限制。
// 2. 每个批次都被单独编码，并输出到单独的文件中。
// 3. 用户可以指定每个分片的大小，这决定了每个批次可以处理的数据量。
// 4. 输出文件的命名格式为 原文件名.batch批次号.shard分片号，便于后续解码。
// 5. 它保持了原始 stream-encoder.go 的大部分功能，如命令行参数解析和错误处理。
// 要使用这个编码器，您可以这样编译和运行：
//
// go build large-file-encoder.go
// ./large-file-encoder -data 17 -par 3 -size 1048576 大文件.bin
//
// 这将把 大文件.bin 分成多个批次，每个批次有17个数据分片和3个奇偶校验分片，每个分片大小为1MB。
// 请注意，这个实现需要相应的解码器来处理这种批次编码的文件。解码器应该能够识别批次结构，并正确地重建原始文件。

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/reedsolomon"
	logging "github.com/dep2p/log"
)

var logger = logging.Logger("examples")
var (
	// 定义数据分片数量的命令行标志，默认值为17
	dataShards = flag.Int("data", 17, "每批数据分片数量")
	// 定义奇偶校验分片数量的命令行标志，默认值为3
	parShards = flag.Int("par", 3, "每批奇偶校验分片数量")
	// 定义每个分片大小的命令行标志，默认值为1MB
	shardSize = flag.Int64("size", 1024*1024, "每个分片的大小（字节）")
	// 定义输出目录的命令行标志
	outDir = flag.String("out", "", "输出目录")
	// 定义Reed-Solomon编码的最大批次大小常量
	maxBatchSize = 256 // Reed-Solomon 编码的最大批次大小
)

// init 初始化函数
// 设置命令行使用说明
func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "使用方法: %s [-flags] filename.ext\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "有效的标志:\n")
		flag.PrintDefaults()
	}
}

// main 主函数
// 解析命令行参数，打开输入文件，创建编码器，并处理文件
func main() {
	// 解析命令行参数
	flag.Parse()
	// 检查是否提供了输入文件名
	if flag.NArg() != 1 {
		logger.Error("未提供输入文件名")
		flag.Usage()
		os.Exit(1)
	}

	// 检查分片数量是否超过最大批次大小
	if *dataShards+*parShards > maxBatchSize {
		logger.Errorf("数据分片和奇偶校验分片的总和不能超过 %d", maxBatchSize)
		os.Exit(1)
	}

	// 获取输入文件名
	inputFile := flag.Arg(0)
	logger.Infof("正在打开 %s", inputFile)
	// 打开输入文件
	f, err := os.Open(inputFile)
	if err != nil {
		logger.Errorf("打开文件失败: %v", err)
		os.Exit(1)
	}
	defer f.Close()

	// 获取文件信息
	fileInfo, err := f.Stat()
	if err != nil {
		logger.Errorf("获取文件信息失败: %v", err)
		os.Exit(1)
	}
	fileSize := fileInfo.Size()

	// 创建Reed-Solomon编码器
	enc, err := reedsolomon.NewStream(*dataShards, *parShards)
	if err != nil {
		logger.Errorf("创建Reed-Solomon编码器失败: %v", err)
		os.Exit(1)
	}

	// 计算需要处理的批次数
	batchCount := (fileSize + *shardSize*int64(*dataShards) - 1) / (int64(*dataShards) * *shardSize)
	logger.Infof("文件大小: %d 字节, 将被分成 %d 批次处理", fileSize, batchCount)

	// 循环处理每个批次
	for batch := int64(0); batch < batchCount; batch++ {
		err := processBatch(f, enc, batch, fileSize, inputFile)
		if err != nil {
			logger.Errorf("处理批次 %d 失败: %v", batch, err)
			os.Exit(1)
		}
	}

	logger.Info("编码完成")
}

// processBatch 处理单个批次的编码
// 参数:
//   - f: 输入文件
//   - enc: Reed-Solomon编码器
//   - batch: 当前批次号
//   - fileSize: 输入文件总大小
//   - inputFile: 输入文件名
//
// 返回值:
//   - error: 处理过程中的错误
func processBatch(f *os.File, enc reedsolomon.StreamEncoder, batch int64, fileSize int64, inputFile string) error {
	logger.Infof("正在处理批次 %d", batch+1)

	// 计算当前批次的大小
	batchSize := *shardSize * int64(*dataShards)
	if (batch+1)*batchSize > fileSize {
		batchSize = fileSize - batch*batchSize
	}

	// 创建此批次的输出文件
	outFiles := make([]*os.File, *dataShards+*parShards)
	for i := range outFiles {
		// 生成输出文件名
		outName := fmt.Sprintf("%s.batch%d.shard%d", filepath.Base(inputFile), batch, i)
		if *outDir != "" {
			outName = filepath.Join(*outDir, outName)
		}
		// 创建输出文件
		outFile, err := os.Create(outName)
		if err != nil {
			return fmt.Errorf("创建输出文件 %s 失败: %v", outName, err)
		}
		defer outFile.Close()
		outFiles[i] = outFile
	}

	// 准备数据分片的写入器
	dataWriters := make([]io.Writer, *dataShards)
	for i := range dataWriters {
		dataWriters[i] = outFiles[i]
	}

	// 将文件指针移动到当前批次的起始位置
	// Seek 将文件上下一次读取或写入的偏移量设置为 offset，根据 whence 进行解释：0 表示相对于文件原点，1 表示相对于当前偏移量，2 表示相对于末尾。
	// 它返回新的偏移量和错误（如果有）。
	// 未指定使用 O_APPEND 打开的文件上的 Seek 行为。
	_, err := f.Seek(batch*batchSize, io.SeekStart)
	if err != nil {
		return fmt.Errorf("移动文件指针失败: %v", err)
	}
	// 分割数据到各个数据分片
	// LimitReader 返回一个从 r 读取但在 n 个字节后以 EOF 停止的 Reader。
	// 底层实现是 *LimitedReader。
	err = enc.Split(io.LimitReader(f, batchSize), dataWriters, batchSize)
	if err != nil {
		return fmt.Errorf("分割数据失败: %v", err)
	}

	// 准备用于编码的输入读取器
	dataReaders := make([]io.Reader, *dataShards)
	for i, file := range outFiles[:*dataShards] {
		// 将文件指针移动到文件开头
		_, err := file.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("移动文件指针到开头失败: %v", err)
		}
		dataReaders[i] = file
	}

	// 准备奇偶校验写入器
	parityWriters := make([]io.Writer, *parShards)
	for i := range parityWriters {
		parityWriters[i] = outFiles[*dataShards+i]
	}

	// 编码奇偶校验数据
	err = enc.Encode(dataReaders, parityWriters)
	if err != nil {
		return fmt.Errorf("编码奇偶校验数据失败: %v", err)
	}

	logger.Infof("批次 %d 已分割为 %d 个数据分片 + %d 个奇偶校验分片", batch+1, *dataShards, *parShards)
	return nil
}
