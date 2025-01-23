//go:build ignore
// +build ignore

// Copyright 2015, Klaus Post, see LICENSE for details.
//
// 简单解码器示例。
//
// 该解码器反转了 "simple-encoder.go" 的过程。
//
// 要构建可执行文件，请使用:
//
// go build simple-decoder.go
//
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
	"os"

	"github.com/bpfs/defs/v2/reedsolomon"
)

// 定义数据分片数量的命令行标志
var dataShards = flag.Int("data", 4, "Number of shards to split the data into")

// 定义奇偶校验分片数量的命令行标志
var parShards = flag.Int("par", 2, "Number of parity shards")

// 定义可选输出文件路径的命令行标志
var outFile = flag.String("out", "", "Alternative output path/file")

func init() {
	// 设置命令行用法说明
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  simple-decoder [-flags] basefile.ext\nDo not add the number to the filename.\n")
		fmt.Fprintf(os.Stderr, "Valid flags:\n")
		flag.PrintDefaults()
	}
}

func main() {
	// 解析命令行参数
	flag.Parse()
	args := flag.Args()
	// 检查是否提供了输入文件名
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Error: No filenames given\n")
		flag.Usage()
		os.Exit(1)
	}
	fname := args[0]

	// 创建编码矩阵
	enc, err := reedsolomon.New(*dataShards, *parShards)
	checkErr(err)

	// 创建分片并加载数据
	shards := make([][]byte, *dataShards+*parShards)
	for i := range shards {
		infn := fmt.Sprintf("%s.%d", fname, i)
		fmt.Println("Opening", infn)
		shards[i], err = os.ReadFile(infn)
		if err != nil {
			fmt.Println("Error reading file", err)
			shards[i] = nil
		}
	}

	// 验证分片
	ok, err := enc.Verify(shards)
	if ok {
		fmt.Println("No reconstruction needed")
	} else {
		fmt.Println("Verification failed. Reconstructing data")
		// 重建数据
		err = enc.Reconstruct(shards)
		if err != nil {
			fmt.Println("Reconstruct failed -", err)
			os.Exit(1)
		}
		// 再次验证
		ok, err = enc.Verify(shards)
		if !ok {
			fmt.Println("Verification failed after reconstruction, data likely corrupted.")
			os.Exit(1)
		}
		checkErr(err)
	}

	// 合并分片并写入文件
	outfn := *outFile
	if outfn == "" {
		outfn = fname
	}

	fmt.Println("Writing data to", outfn)
	f, err := os.Create(outfn)
	checkErr(err)

	// 我们不知道确切的文件大小
	err = enc.Join(f, shards, len(shards[0])**dataShards)
	checkErr(err)
}

// 检查错误并在出现错误时退出程序
func checkErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		os.Exit(2)
	}
}
