//go:build ignore
// +build ignore

// Copyright 2015, Klaus Post, see LICENSE for details.

// 简单的流编码器示例

// 编码器将单个文件编码为多个分片
// 要反转此过程，请参见 "stream-decoder.go"

// 要构建可执行文件，请使用:
//
// go build stream-encoder.go

// 简单编码器/解码器的缺点:
// * 如果输入文件大小不能被数据分片数整除，输出将包含额外的零
// * 如果解码器的分片数与编码器不同，将生成无效输出
// * 如果分片中的值发生变化，无法重建
// * 如果两个分片被交换，重建将始终失败
//   您需要按照给定的顺序提供分片

// 解决方案是保存包含以下内容的元数据文件:
//
// * 文件大小
// * 数据/奇偶校验分片的数量
// * 每个分片的哈希值
// * 分片的顺序
//
// 如果保存这些属性，您应该能够检测分片中的文件损坏
// 并在剩余所需数量的分片的情况下重建数据

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/v2/reedsolomon"
)

// 定义数据分片数量的命令行标志
var dataShards = flag.Int("data", 4, "要将数据分割成的分片数量，必须小于257。")

// 定义奇偶校验分片数量的命令行标志
var parShards = flag.Int("par", 2, "奇偶校验分片的数量")

// 定义可选输出目录的命令行标志
var outDir = flag.String("out", "", "可选的输出目录")

func init() { // 初始化函数
	// 设置命令行用法说明
	flag.Usage = func() { // 定义Usage函数
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])                 // 打印用法说明
		fmt.Fprintf(os.Stderr, "  %s [-flags] filename.ext\n\n", os.Args[0]) // 打印示例
		fmt.Fprintf(os.Stderr, "Valid flags:\n")                             // 打印有效标志提示
		flag.PrintDefaults()                                                 // 打印默认标志
	}
}

func main() { // 主函数
	// 解析命令行参数
	flag.Parse()        // 解析命令行参数
	args := flag.Args() // 获取非标志参数
	// 检查是否提供了输入文件名
	if len(args) != 1 { // 如果参数数量不为1
		fmt.Fprintf(os.Stderr, "错误：未提供输入文件名\n") // 打印错误信息
		flag.Usage()                            // 打印用法
		os.Exit(1)                              // 退出程序
	}
	// 检查数据分片和奇偶校验分片的总数是否超过256
	if (*dataShards + *parShards) > 256 { // 如果总分片数超过256
		fmt.Fprintf(os.Stderr, "错误：数据分片和奇偶校验分片的总和不能超过256\n") // 打印错误信息
		os.Exit(1)                                           // 退出程序
	}
	fname := args[0] // 获取输入文件名

	// 步骤1：创建Reed-Solomon编码器
	enc, err := reedsolomon.NewStream(*dataShards, *parShards) // 创建Reed-Solomon编码器
	checkErr(err)                                              // 检查错误

	// 步骤2：打开输入文件
	fmt.Println("正在打开", fname) // 打印正在打开的文件名
	f, err := os.Open(fname)   // 打开文件
	checkErr(err)              // 检查错误

	// 步骤3：获取输入文件的状态信息（主要是文件大小）
	instat, err := f.Stat() // 获取文件状态
	checkErr(err)           // 检查错误

	// 步骤4：计算总分片数并创建输出文件切片
	shards := *dataShards + *parShards // 计算总分片数
	out := make([]*os.File, shards)    // 创建输出文件切片

	// 步骤5：创建结果文件
	dir, file := filepath.Split(fname) // 分割路径和文件名
	if *outDir != "" {                 // 如果指定了输出目录
		dir = *outDir // 使用指定的输出目录
	}
	for i := range out { // 遍历所有分片
		outfn := fmt.Sprintf("%s.%d", file, i)             // 生成输出文件名
		fmt.Println("Creating", outfn)                     // 打印正在创建的文件名
		out[i], err = os.Create(filepath.Join(dir, outfn)) // 创建输出文件
		checkErr(err)                                      // 检查错误
	}

	// 步骤6：准备数据分片的写入器 ([]io.Writer)
	data := make([]io.Writer, *dataShards) // 创建数据分片写入器切片
	for i := range data {                  // 遍历数据分片
		data[i] = out[i] // 设置写入器
	}

	// 步骤7：执行分片操作
	err = enc.Split(f, data, instat.Size()) // 分割输入文件
	checkErr(err)                           // 检查错误

	// 关闭并重新打开文件
	input := make([]io.Reader, *dataShards) // 创建输入读取器切片

	for i := range data { // 遍历数据分片
		out[i].Close()                   // 关闭输出文件
		f, err := os.Open(out[i].Name()) // 重新打开文件
		checkErr(err)                    // 检查错误
		input[i] = f                     // 设置读取器
		defer f.Close()                  // 延迟关闭文件
	}

	// 步骤9：创建奇偶校验输出写入器 ([]io.Writer)
	parity := make([]io.Writer, *parShards) // 创建奇偶校验写入器切片
	for i := range parity {                 // 遍历奇偶校验分片
		parity[i] = out[*dataShards+i]   // 设置写入器
		defer out[*dataShards+i].Close() // 延迟关闭文件
	}

	// 步骤10：编码奇偶校验
	err = enc.Encode(input, parity)                                                      // 编码奇偶校验
	checkErr(err)                                                                        // 检查错误
	fmt.Printf("File split into %d data + %d parity shards.\n", *dataShards, *parShards) // 打印分片信息
}

// 检查错误并在出现错误时退出程序
func checkErr(err error) { // 定义错误检查函数
	if err != nil { // 如果有错误
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error()) // 打印错误信息
		os.Exit(2)                                       // 退出程序
	}
}
