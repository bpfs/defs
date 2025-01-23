//go:build ignore
// +build ignore

// 版权所有 2015, Klaus Post, 详见 LICENSE 文件。
//
// 流解码器示例。
//
// 该解码器反转了 "stream-encoder.go" 的过程。
//
// 要构建可执行文件，请使用:
//
// go build stream-decoder.go
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

package main // 定义主包

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/bpfs/defs/v2/reedsolomon" // 导入所需的包
)

// 定义数据分片数量的命令行标志
var dataShards = flag.Int("data", 4, "将数据分割成的分片数量")

// 定义奇偶校验分片数量的命令行标志
var parShards = flag.Int("par", 2, "奇偶校验分片的数量")

// 定义可选输出文件路径的命令行标志
var outFile = flag.String("out", "", "可选的输出路径/文件")

func init() { // 初始化函数
	// 设置命令行用法说明
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])                           // 打印用法说明
		fmt.Fprintf(os.Stderr, "  %s [-flags] 基础文件名.扩展名\n请不要在文件名中添加数字。\n", os.Args[0]) // 打印示例
		fmt.Fprintf(os.Stderr, "Valid flags:\n")                                       // 打印有效标志提示
		flag.PrintDefaults()                                                           // 打印默认标志
	}
}

func main() { // 主函数
	// 解析命令行参数
	flag.Parse()
	args := flag.Args()
	// 检查是否提供了输入文件名
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "错误：未提供文件名\n") // 打印错误信息
		flag.Usage()                          // 打印用法
		os.Exit(1)                            // 退出程序
	}
	fname := args[0] // 获取文件名

	// 创建编码矩阵
	enc, err := reedsolomon.NewStream(*dataShards, *parShards)
	checkErr(err) // 检查错误

	// 打开输入文件
	shards, size, err := openInput(*dataShards, *parShards, fname)
	checkErr(err) // 检查错误

	// 验证分片
	ok, err := enc.Verify(shards)
	if ok {
		fmt.Println("无需重建") // 打印无需重建的信息
	} else {
		fmt.Println("验证失败。正在重建数据") // 打印验证失败，开始重建的信息
		// 重新打开输入文件
		shards, size, err = openInput(*dataShards, *parShards, fname)
		checkErr(err) // 检查错误
		// 创建输出目标写入器
		out := make([]io.Writer, len(shards))
		for i := range out {
			if shards[i] == nil {
				outfn := fmt.Sprintf("%s.%d", fname, i) // 生成输出文件名
				fmt.Println("Creating", outfn)          // 打印创建文件的信息
				out[i], err = os.Create(outfn)          // 创建输出文件
				checkErr(err)                           // 检查错误
			}
		}
		// 重建数据
		err = enc.Reconstruct(shards, out)
		if err != nil {
			fmt.Println("重建失败 -", err) // 打印重建失败的信息
			os.Exit(1)                 // 退出程序
		}
		// 关闭输出文件
		for i := range out {
			if out[i] != nil {
				err := out[i].(*os.File).Close() // 关闭文件
				checkErr(err)                    // 检查错误
			}
		}
		// 重新打开输入文件并验证
		shards, size, err = openInput(*dataShards, *parShards, fname)
		ok, err = enc.Verify(shards)
		if !ok {
			fmt.Println("重建后验证失败，数据可能已损坏:", err) // 打印验证失败的信息
			os.Exit(1)                           // 退出程序
		}
		checkErr(err) // 检查错误
	}

	// 设置输出文件名
	outfn := *outFile
	if outfn == "" {
		outfn = fname // 如果未指定输出文件名，使用输入文件名
	}

	// 创建输出文件
	fmt.Println("正在将数据写入", outfn) // 打印写入数据的信息
	f, err := os.Create(outfn)    // 创建输出文件
	checkErr(err)                 // 检查错误

	// 重新打开输入文件
	shards, size, err = openInput(*dataShards, *parShards, fname)
	checkErr(err) // 检查错误

	// 合并分片并写入输出文件
	err = enc.Join(f, shards, int64(*dataShards)*size)
	checkErr(err) // 检查错误
}

// 打开输入文件并返回分片读取器、文件大小和错误
func openInput(dataShards, parShards int, fname string) (r []io.Reader, size int64, err error) {
	// 创建分片并加载数据
	shards := make([]io.Reader, dataShards+parShards)
	for i := range shards {
		infn := fmt.Sprintf("%s.%d", fname, i) // 生成输入文件名
		fmt.Println("Opening", infn)           // 打印打开文件的信息
		f, err := os.Open(infn)                // 打开文件
		if err != nil {
			fmt.Println("Error reading file", err) // 打印读取文件错误的信息
			shards[i] = nil                        // 设置分片为nil
			continue                               // 继续下一个循环
		} else {
			shards[i] = f // 设置分片为文件
		}
		stat, err := f.Stat() // 获取文件信息
		checkErr(err)         // 检查错误
		if stat.Size() > 0 {
			size = stat.Size() // 设置文件大小
		} else {
			shards[i] = nil // 如果文件为空，设置分片为nil
		}
	}
	return shards, size, nil // 返回分片、大小和错误
}

// 检查错误并在出现错误时退出程序
func checkErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error()) // 打印错误信息
		os.Exit(2)                                       // 退出程序
	}
}
