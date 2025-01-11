package reedsolomon_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"

	"github.com/bpfs/defs/v2/reedsolomon"
)

// fillRandom 用随机数据填充字节切片
// 参数:
//   - p: 要填充的字节切片
func fillRandom(p []byte) {
	for i := 0; i < len(p); i += 7 {
		val := rand.Int63()
		for j := 0; i+j < len(p) && j < 7; j++ {
			p[i+j] = byte(val)
			val >>= 8
		}
	}
}

// ExampleEncoder 演示了如何使用编码器的所有功能
// 注意：为了保持简洁，所有错误检查都已被移除
func ExampleEncoder() {
	// 创建一些示例数据
	var data = make([]byte, 250000)
	fillRandom(data)

	// 创建一个具有17个数据分片和3个奇偶校验分片的编码器
	enc, _ := reedsolomon.New(17, 3)

	// 将数据分割成分片
	shards, _ := enc.Split(data)

	// 编码奇偶校验集
	_ = enc.Encode(shards)

	// 验证奇偶校验集
	ok, _ := enc.Verify(shards)
	if ok {
		fmt.Println("ok")
	}

	// 删除两个分片
	shards[10], shards[11] = nil, nil

	// 重建分片
	_ = enc.Reconstruct(shards)

	// 验证数据集
	ok, _ = enc.Verify(shards)
	if ok {
		fmt.Println("ok")
	}
	// Output: ok
	// ok
}

// ExampleEncoder_EncodeIdx 演示了如何使用EncoderIdx的所有功能
// 注意：为了保持简洁，所有错误检查都已被移除
func ExampleEncoder_EncodeIdx() {
	const dataShards = 7
	const erasureShards = 3

	// 创建一些示例数据
	var data = make([]byte, 250000)
	fillRandom(data)

	// 创建一个具有7个数据分片和3个奇偶校验分片的编码器
	enc, _ := reedsolomon.New(dataShards, erasureShards)

	// 将数据分割成分片
	shards, _ := enc.Split(data)

	// 将擦除分片置零
	for i := 0; i < erasureShards; i++ {
		clear := shards[dataShards+i]
		for j := range clear {
			clear[j] = 0
		}
	}

	for i := 0; i < dataShards; i++ {
		// 一次编码一个分片
		// 注意这如何提供线性访问
		// 但是分片不需要按顺序传递
		// 每次运行都会更新所有奇偶校验分片
		_ = enc.EncodeIdx(shards[i], i, shards[dataShards:])
	}

	// 验证奇偶校验集
	ok, err := enc.Verify(shards)
	if ok {
		fmt.Println("ok")
	} else {
		fmt.Println(err)
	}

	// 删除两个分片
	shards[dataShards-2], shards[dataShards-2] = nil, nil

	// 重建分片
	_ = enc.Reconstruct(shards)

	// 验证数据集
	ok, err = enc.Verify(shards)
	if ok {
		fmt.Println("ok")
	} else {
		fmt.Println(err)
	}
	// Output: ok
	// ok
}

// ExampleEncoder_slicing 演示了分片可以被任意切片和合并，并且仍然保持有效
func ExampleEncoder_slicing() {
	// 创建一些示例数据
	var data = make([]byte, 250000)
	fillRandom(data)

	// 创建5个各包含50000个元素的数据分片
	enc, _ := reedsolomon.New(5, 3)
	shards, _ := enc.Split(data)
	err := enc.Encode(shards)
	if err != nil {
		panic(err)
	}

	// 检查是否验证通过
	ok, err := enc.Verify(shards)
	if ok && err == nil {
		fmt.Println("encode ok")
	}

	// 将50000个元素的数据集分割成两个25000个元素的集合
	splitA := make([][]byte, 8)
	splitB := make([][]byte, 8)

	// 合并成一个100000个元素的集合
	merged := make([][]byte, 8)

	// 分割/合并分片
	for i := range shards {
		splitA[i] = shards[i][:25000]
		splitB[i] = shards[i][25000:]

		// 将其与自身连接
		merged[i] = append(make([]byte, 0, len(shards[i])*2), shards[i]...)
		merged[i] = append(merged[i], shards[i]...)
	}

	// 每个部分应该仍然验证为ok
	ok, err = enc.Verify(shards)
	if ok && err == nil {
		fmt.Println("splitA ok")
	}

	ok, err = enc.Verify(splitB)
	if ok && err == nil {
		fmt.Println("splitB ok")
	}

	ok, err = enc.Verify(merged)
	if ok && err == nil {
		fmt.Println("merge ok")
	}
	// Output: encode ok
	// splitA ok
	// splitB ok
	// merge ok
}

// ExampleEncoder_xor 演示了分片可以进行异或操作并且仍然保持有效集合
//
// 每个分片中的第'n'个元素的异或值必须相同，
// 除非你与类似大小的编码分片集进行异或
func ExampleEncoder_xor() {
	// 创建一些示例数据
	var data = make([]byte, 25000)
	fillRandom(data)

	// 创建5个各包含5000个元素的数据分片
	enc, _ := reedsolomon.New(5, 3)
	shards, _ := enc.Split(data)
	err := enc.Encode(shards)
	if err != nil {
		panic(err)
	}

	// 检查是否验证通过
	ok, err := enc.Verify(shards)
	if !ok || err != nil {
		fmt.Println("falied initial verify", err)
	}

	// 创建一个异或后的集合
	xored := make([][]byte, 8)

	// 我们按索引进行异或，所以你可以看到异或可以改变，
	// 但是它应该在你的分片中垂直保持恒定
	for i := range shards {
		xored[i] = make([]byte, len(shards[i]))
		for j := range xored[i] {
			xored[i][j] = shards[i][j] ^ byte(j&0xff)
		}
	}

	// 每个部分应该仍然验证为ok
	ok, err = enc.Verify(xored)
	if ok && err == nil {
		fmt.Println("verified ok after xor")
	}
	// Output: verified ok after xor
}

// ExampleStreamEncoder 展示了一个简单的流编码器，我们从包含每个分片的读取器的[]io.Reader中进行编码
//
// 输入和输出可以与文件、网络流或适合你需求的任何东西交换
func ExampleStreamEncoder() {
	dataShards := 5
	parityShards := 2

	// 创建一个具有指定数据和奇偶校验分片数量的StreamEncoder
	rs, err := reedsolomon.NewStream(dataShards, parityShards)
	if err != nil {
		log.Fatal(err)
	}

	shardSize := 50000

	// 创建输入数据分片
	input := make([][]byte, dataShards)
	for s := range input {
		input[s] = make([]byte, shardSize)
		fillRandom(input[s])
	}

	// 将我们的缓冲区转换为io.Readers
	readers := make([]io.Reader, dataShards)
	for i := range readers {
		readers[i] = io.Reader(bytes.NewBuffer(input[i]))
	}

	// 创建我们的输出io.Writers
	out := make([]io.Writer, parityShards)
	for i := range out {
		out[i] = ioutil.Discard
	}

	// 从输入编码到输出
	err = rs.Encode(readers, out)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("ok")
	// OUTPUT: ok
}
