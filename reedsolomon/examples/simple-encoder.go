//go:build ignore
// +build ignore

// Copyright 2015, Klaus Post, see LICENSE for details.

// 简单编码器示例

// 编码器将单个文件编码为多个分片
// 要反转此过程，请参见 "simpledecoder.go"

// 要构建可执行文件，请使用:
//
// go build simple-decoder.go

// 简单编码器/解码器的缺点:
// * 如果输入文件大小不能被数据分片数整除，输出将包含额外的零
//
// * 如果解码器的分片数与编码器不同，将生成无效输出
//
// * 如果分片中的值发生变化，无法重建
//
// * 如果两个分片被交换，重建将始终失败
//   您需要按照给定的顺序提供分片
//
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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/reedsolomon"
)

var dataShards = flag.Int("data", 4, "Number of shards to split the data into, must be below 257.") // 定义数据分片数量的命令行标志
var parShards = flag.Int("par", 2, "Number of parity shards")                                       // 定义奇偶校验分片数量的命令行标志
var outDir = flag.String("out", "", "Alternative output directory")                                 // 定义可选输出目录的命令行标志

func init() { // 初始化函数
	flag.Usage = func() { // 设置命令行用法说明
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])                 // 打印用法说明
		fmt.Fprintf(os.Stderr, "  simple-encoder [-flags] filename.ext\n\n") // 打印命令格式
		fmt.Fprintf(os.Stderr, "Valid flags:\n")                             // 打印有效标志提示
		flag.PrintDefaults()                                                 // 打印默认标志
	}
}

func main() { // 主函数
	flag.Parse()        // 解析命令行参数
	args := flag.Args() // 获取非标志参数
	if len(args) != 1 { // 检查是否提供了输入文件名
		fmt.Fprintf(os.Stderr, "Error: No input filename given\n") // 打印错误信息
		flag.Usage()                                               // 显示用法说明
		os.Exit(1)                                                 // 退出程序
	}
	if (*dataShards + *parShards) > 256 { // 检查数据分片和奇偶校验分片的总数是否超过256
		fmt.Fprintf(os.Stderr, "Error: sum of data and parity shards cannot exceed 256\n") // 打印错误信息
		os.Exit(1)                                                                         // 退出程序
	}
	fname := args[0] // 获取输入文件名

	enc, err := reedsolomon.New(*dataShards, *parShards) // 创建编码矩阵
	checkErr(err)                                        // 检查错误

	fmt.Println("Opening", fname)    // 打印正在打开的文件名
	b, err := ioutil.ReadFile(fname) // 读取整个文件内容
	checkErr(err)                    // 检查错误

	shards, err := enc.Split(b)                                                                             // 将文件分割成等大小的分片
	checkErr(err)                                                                                           // 检查错误
	fmt.Printf("File split into %d data+parity shards with %d bytes/shard.\n", len(shards), len(shards[0])) // 打印分片信息

	err = enc.Encode(shards) // 编码奇偶校验
	checkErr(err)            // 检查错误

	dir, file := filepath.Split(fname) // 分离文件路径和文件名
	if *outDir != "" {                 // 如果指定了输出目录
		dir = *outDir // 使用指定的输出目录
	}
	for i, shard := range shards { // 遍历所有分片
		outfn := fmt.Sprintf("%s.%d", file, i) // 生成输出文件名

		fmt.Println("Writing to", outfn)                               // 打印正在写入的文件名
		err = ioutil.WriteFile(filepath.Join(dir, outfn), shard, 0644) // 写入分片到文件
		checkErr(err)                                                  // 检查错误
	}
}

func checkErr(err error) { // 错误检查函数
	if err != nil { // 如果有错误
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error()) // 打印错误信息
		os.Exit(2)                                       // 退出程序
	}
}
