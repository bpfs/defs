/**
 * 基于8位值的Reed-Solomon编码
 *
 * Copyright 2015, Klaus Post
 * Copyright 2015, Backblaze, Inc.
 */

// Package reedsolomon 提供Go语言的纠删码功能
//
// 使用方法和示例请参考 https://github.com/klauspost/reedsolomon
package reedsolomon

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"

	"github.com/klauspost/cpuid/v2"
)

// Encoder 是用于对数据进行Reed-Solomon奇偶校验编码的接口
type Encoder interface {
	// Encode 为一组数据分片生成奇偶校验
	// 参数:
	//   - shards: 包含数据分片和奇偶校验分片的切片数组
	// 返回值:
	//   - error: 编码过程中的错误信息
	// 说明:
	// - 分片数量必须与New()函数指定的数量匹配
	// - 每个分片都是字节数组,且大小必须相同
	// - 奇偶校验分片会被覆盖,数据分片保持不变
	// - 在编码过程中可以安全地读取数据分片
	Encode(shards [][]byte) error

	// EncodeIdx 为单个数据分片添加奇偶校验
	// 参数:
	//   - dataShard: 数据分片
	//   - idx: 分片索引
	//   - parity: 奇偶校验分片数组
	// 返回值:
	//   - error: 编码过程中的错误信息
	// 说明:
	// - 奇偶校验分片初始值应为0,调用者必须将其置零
	// - 数据分片必须只传递一次,不会对此进行检查
	// - 奇偶校验分片会被更新,数据分片保持不变
	EncodeIdx(dataShard []byte, idx int, parity [][]byte) error

	// Verify 验证奇偶校验分片是否包含正确的数据
	// 参数:
	//   - shards: 包含数据分片和奇偶校验分片的切片数组
	// 返回值:
	//   - bool: 验证是否通过
	//   - error: 验证过程中的错误信息
	// 说明:
	// - 数据格式与Encode相同
	// - 不会修改任何数据,可以在验证过程中安全读取
	Verify(shards [][]byte) (bool, error)

	// Reconstruct 尝试重建丢失的分片
	// 参数:
	//   - shards: 包含部分数据的分片数组
	// 返回值:
	//   - error: 重建过程中的错误信息
	// 说明:
	// - 数组长度必须等于分片总数
	// - 通过将分片设置为nil或零长度来表示丢失的分片
	// - 如果分片容量足够,将使用现有内存,否则分配新内存
	// - 如果可用分片太少,将返回ErrTooFewShards错误
	// - 重建后的分片集合是完整的,但未验证完整性
	Reconstruct(shards [][]byte) error

	// ReconstructData 仅重建丢失的数据分片
	// 参数:
	//   - shards: 包含部分数据的分片数组
	// 返回值:
	//   - error: 重建过程中的错误信息
	// 说明:
	// - 数组长度必须等于分片数
	// - 通过将分片设置为nil或零长度来表示丢失的分片
	// - 如果分片容量足够,将使用现有内存,否则分配新内存
	// - 如果可用分片太少,将返回ErrTooFewShards错误
	// - 由于重建后可能缺少奇偶校验分片,验证可能会失败
	ReconstructData(shards [][]byte) error

	// ReconstructSome 仅重建指定的分片
	// 参数:
	//   - shards: 包含部分数据的分片数组
	//   - required: 指示需要重建的分片的布尔数组
	// 返回值:
	//   - error: 重建过程中的错误信息
	// 说明:
	// - required数组长度必须等于分片总数或数据分片数
	// - 如果长度等于数据分片数,将忽略奇偶校验分片的重建
	// - shards数组长度必须等于分片总数
	ReconstructSome(shards [][]byte, required []bool) error

	// Update 用于更新部分数据分片并重新计算奇偶校验
	// 参数:
	//   - shards: 包含旧数据分片和旧奇偶校验分片的数组
	//   - newDatashards: 更改后的数据分片数组
	// 返回值:
	//   - error: 更新过程中的错误信息
	// 说明:
	// - 新的奇偶校验分片将存储在shards[DataShards:]中
	// - 当数据分片远多于奇偶校验分片且变更较少时,此方法比Encode更快
	Update(shards [][]byte, newDatashards [][]byte) error

	// Split 将数据切片分割成指定数量的分片
	// 参数:
	//   - data: 要分割的数据
	// 返回值:
	//   - [][]byte: 分割后的分片数组
	//   - error: 分割过程中的错误信息
	// 说明:
	// - 数据将被均匀分割
	// - 如果数据大小不能被分片数整除,最后一个分片将补零
	// - 如果提供的数据切片有额外容量,将用于分配奇偶校验分片
	Split(data []byte) ([][]byte, error)

	// Join 将分片合并并写入目标
	// 参数:
	//   - dst: 写入目标
	//   - shards: 分片数组
	//   - outSize: 期望的输出大小
	// 返回值:
	//   - error: 合并过程中的错误信息
	// 说明:
	// - 仅考虑数据分片
	// - 必须提供准确的输出大小
	// - 如果分片数量不足,将返回ErrTooFewShards错误
	// - 如果总数据大小小于outSize,将返回ErrShortData错误
	Join(dst io.Writer, shards [][]byte, outSize int) error
}

// Extensions 是一个可选接口
// 所有返回的实例都将支持此接口
type Extensions interface {
	// ShardSizeMultiple 返回分片大小必须是其倍数的值
	ShardSizeMultiple() int

	// DataShards 返回数据分片的数量
	DataShards() int

	// ParityShards 返回奇偶校验分片的数量
	ParityShards() int

	// TotalShards 返回分片总数
	TotalShards() int

	// AllocAligned 分配TotalShards数量的对齐内存切片
	// 参数:
	//   - each: 每个分片的大小
	// 返回值:
	//   - [][]byte: 分配的内存切片数组
	AllocAligned(each int) [][]byte
}

const (
	// codeGenMinSize 是代码生成的最小大小
	codeGenMinSize = 64
	// codeGenMinShards 是代码生成的最小分片数
	codeGenMinShards = 3
	// gfniCodeGenMaxGoroutines 是GFNI代码生成使用的最大goroutine数
	gfniCodeGenMaxGoroutines = 4

	// intSize 是当前平台的整数位数(32或64位)
	intSize = 32 << (^uint(0) >> 63) // 32 or 64
	// maxInt 是当前平台的最大整数值
	maxInt = 1<<(intSize-1) - 1
)

// reedSolomon 包含用于特定数据分片和奇偶校验分片分布的矩阵
// 通过 New() 函数构造实例
type reedSolomon struct {
	dataShards   int            // 数据分片数量,不应修改
	parityShards int            // 奇偶校验分片数量,不应修改
	totalShards  int            // 分片总数,由计算得出,不应修改
	m            matrix         // 编码矩阵,用于生成奇偶校验数据
	tree         *inversionTree // 反转树,用于优化恢复过程
	parity       [][]byte       // 奇偶校验数据缓存
	o            options        // 编码器配置选项
	mPoolSz      int            // 矩阵池大小
	mPool        sync.Pool      // 临时矩阵等对象的内存池,用于减少内存分配
}

// 确保 reedSolomon 实现了 Extensions 接口
var _ = Extensions(&reedSolomon{})

// ShardSizeMultiple 返回分片大小必须是其倍数的值
// 对于基本的Reed-Solomon实现,返回1表示没有特殊的对齐要求
func (r *reedSolomon) ShardSizeMultiple() int {
	return 1
}

// DataShards 返回数据分片的数量
// 这个值在创建编码器时指定,之后不会改变
func (r *reedSolomon) DataShards() int {
	return r.dataShards
}

// ParityShards 返回奇偶校验分片的数量
// 这个值在创建编码器时指定,之后不会改变
func (r *reedSolomon) ParityShards() int {
	return r.parityShards
}

// TotalShards 返回分片总数
// 总分片数等于数据分片数加上奇偶校验分片数
func (r *reedSolomon) TotalShards() int {
	return r.totalShards
}

// AllocAligned 分配指定数量的对齐内存切片
// 参数:
//   - each: 每个分片的大小(字节)
//
// 返回值:
//   - [][]byte: 包含totalShards个切片的数组,每个切片大小为each字节
func (r *reedSolomon) AllocAligned(each int) [][]byte {
	return AllocAligned(r.totalShards, each)
}

// ErrInvShardNum 在以下情况下由 New() 函数返回:
// - 尝试创建数据分片数小于1的编码器
// - 尝试创建奇偶校验分片数小于0的编码器
// 说明:
// - 数据分片数必须大于等于1,因为至少需要1个数据分片来存储原始数据
// - 奇偶校验分片数必须大于等于0,表示可以不使用纠错功能
var ErrInvShardNum = errors.New("cannot create Encoder with less than one data shard or less than zero parity shards")

// ErrMaxShardNum 在以下情况下由 New() 函数返回:
// - 尝试创建数据分片数+奇偶校验分片数超过256的编码器
// 说明:
// - 由于使用GF(2^8)有限域,分片总数不能超过2^8=256
// - 这是Reed-Solomon编码的数学特性决定的限制
var ErrMaxShardNum = errors.New("cannot create Encoder with more than 256 data+parity shards")

// ErrNotSupported 在操作不被支持时返回
// 说明:
// - 当尝试执行编码器不支持的操作时返回此错误
// - 例如在不支持的平台上使用特定的SIMD优化
var ErrNotSupported = errors.New("operation not supported")

// buildMatrix 构建用于编码的矩阵
// 参数:
//   - dataShards: 数据分片数量
//   - totalShards: 总分片数量(数据分片+校验分片)
//
// 返回值:
//   - matrix: 构建的编码矩阵
//   - error: 构建过程中的错误信息
//
// 说明:
// - 矩阵的顶部方阵保证是单位矩阵
// - 这确保了编码后数据分片保持不变
// - 任意行的方阵子集都是可逆的
func buildMatrix(dataShards, totalShards int) (matrix, error) {
	// 首先构建范德蒙德矩阵
	// 理论上这个矩阵可以工作,但不具有数据分片不变的特性
	vm, err := vandermonde(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	// 提取矩阵顶部的方阵部分
	// 大小为 dataShards x dataShards
	top, err := vm.SubMatrix(0, 0, dataShards, dataShards)
	if err != nil {
		return nil, err
	}

	// 计算顶部方阵的逆矩阵
	topInv, err := top.Invert()
	if err != nil {
		return nil, err
	}

	// 将原矩阵乘以顶部方阵的逆矩阵
	// 这样可以使顶部变为单位矩阵,同时保持任意方阵子集可逆的特性
	return vm.Multiply(topInv)
}

// buildMatrixJerasure 创建与Jerasure库相同的编码矩阵
// 参数:
//   - dataShards: 数据分片数量
//   - totalShards: 总分片数量(数据分片+校验分片)
//
// 返回值:
//   - matrix: 构建的编码矩阵
//   - error: 构建过程中的错误信息
//
// 说明:
// - 矩阵的顶部方阵保证是单位矩阵
// - 这确保了编码后数据分片保持不变
func buildMatrixJerasure(dataShards, totalShards int) (matrix, error) {
	// 首先构建范德蒙德矩阵
	// 理论上这个矩阵可以工作,但不具有数据分片不变的特性
	vm, err := vandermonde(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	// Jerasure的特殊处理:
	// 第一行总是 100..00
	vm[0][0] = 1
	for i := 1; i < dataShards; i++ {
		vm[0][i] = 0
	}
	// 最后一行总是 000..01
	for i := 0; i < dataShards-1; i++ {
		vm[totalShards-1][i] = 0
	}
	vm[totalShards-1][dataShards-1] = 1

	// 对每一列进行处理
	for i := 0; i < dataShards; i++ {
		// 找到第i列中非0元素所在的行
		r := i
		for ; r < totalShards && vm[r][i] == 0; r++ {
		}
		// 如果非0元素不在对角线位置,进行行交换
		if r != i {
			t := vm[r]
			vm[r] = vm[i]
			vm[i] = t
		}
		// 通过矩阵运算使对角线元素为1,其他元素为0
		if vm[i][i] != 1 {
			// 将第i列除以vm[i][i]使对角线元素为1
			tmp := galOneOver(vm[i][i])
			for j := 0; j < totalShards; j++ {
				vm[j][i] = galMultiply(vm[j][i], tmp)
			}
		}
		// 消除第i行以外的其他行在第i列的非零元素
		for j := 0; j < dataShards; j++ {
			tmp := vm[i][j]
			if j != i && tmp != 0 {
				for r := 0; r < totalShards; r++ {
					vm[r][j] = galAdd(vm[r][j], galMultiply(tmp, vm[r][i]))
				}
			}
		}
	}

	// 使vm[dataShards]行全为1
	// 通过将每列除以vm[dataShards][j]实现
	for j := 0; j < dataShards; j++ {
		tmp := vm[dataShards][j]
		if tmp != 1 {
			tmp = galOneOver(tmp)
			for i := dataShards; i < totalShards; i++ {
				vm[i][j] = galMultiply(vm[i][j], tmp)
			}
		}
	}

	// 使vm[dataShards...totalShards-1][0]列全为1
	// 通过将每行除以对应的vm[i][0]实现
	for i := dataShards + 1; i < totalShards; i++ {
		tmp := vm[i][0]
		if tmp != 1 {
			tmp = galOneOver(tmp)
			for j := 0; j < dataShards; j++ {
				vm[i][j] = galMultiply(vm[i][j], tmp)
			}
		}
	}

	return vm, nil
}

// buildMatrixPAR1 根据PARv1规范创建编码矩阵
// 参数:
//   - dataShards: 数据分片数量
//   - totalShards: 总分片数量(数据分片+校验分片)
//
// 返回值:
//   - matrix: 生成的编码矩阵
//   - error: 创建过程中的错误
//
// 说明:
//   - 该方法存在缺陷,即使有足够的校验分片,也可能导致无法恢复数据
//   - 矩阵上方为单位矩阵,确保数据分片在编码后保持不变
//   - 矩阵下方为从1开始的转置范德蒙德矩阵
func buildMatrixPAR1(dataShards, totalShards int) (matrix, error) {
	// 创建新矩阵
	result, err := newMatrix(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	// 遍历矩阵的每一行
	for r, row := range result {
		// 如果是数据分片部分(上方)
		if r < dataShards {
			// 设置为单位矩阵(对角线为1)
			result[r][r] = 1
		} else {
			// 如果是校验分片部分(下方)
			// 使用范德蒙德矩阵填充
			for c := range row {
				// 计算范德蒙德矩阵元素:g^((c+1)*(r-dataShards))
				result[r][c] = galExp(byte(c+1), r-dataShards)
			}
		}
	}
	return result, nil
}

// buildMatrixCauchy 根据柯西矩阵创建编码矩阵
// 参数:
//   - dataShards: 数据分片数量
//   - totalShards: 总分片数量(数据分片+校验分片)
//
// 返回值:
//   - matrix: 生成的编码矩阵
//   - error: 创建过程中的错误
//
// 说明:
//   - 矩阵上方为单位矩阵,确保数据分片在编码后保持不变
//   - 矩阵下方为转置的柯西矩阵,用于生成校验分片
func buildMatrixCauchy(dataShards, totalShards int) (matrix, error) {
	// 创建新矩阵
	result, err := newMatrix(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	// 遍历矩阵的每一行
	for r, row := range result {
		// 如果是数据分片部分(上方)
		if r < dataShards {
			// 设置为单位矩阵(对角线为1)
			result[r][r] = 1
		} else {
			// 如果是校验分片部分(下方)
			// 使用柯西矩阵填充
			for c := range row {
				// 使用异或运算和查表计算柯西矩阵元素
				result[r][c] = invTable[(byte(r ^ c))]
			}
		}
	}
	return result, nil
}

// buildXorMatrix 创建纯XOR运算的编码矩阵(仅适用于单个校验分片)
// 参数:
//   - dataShards: 数据分片数量
//   - totalShards: 总分片数量(数据分片+校验分片)
//
// 返回值:
//   - matrix: 生成的编码矩阵
//   - error: 创建过程中的错误
//
// 说明:
//   - 仅支持一个校验分片的情况
//   - 矩阵上方为单位矩阵,确保数据分片在编码后保持不变
//   - 矩阵下方全部为1,实现简单的XOR校验
func buildXorMatrix(dataShards, totalShards int) (matrix, error) {
	// 验证是否只有一个校验分片
	if dataShards+1 != totalShards {
		return nil, errors.New("internal error")
	}

	// 创建新矩阵
	result, err := newMatrix(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	// 遍历矩阵的每一行
	for r, row := range result {
		// 如果是数据分片部分(上方)
		if r < dataShards {
			// 设置为单位矩阵(对角线为1)
			result[r][r] = 1
		} else {
			// 如果是校验分片部分(下方)
			// 所有元素设为1,实现XOR运算
			for c := range row {
				result[r][c] = 1
			}
		}
	}
	return result, nil
}

// New 创建一个新的编码器并初始化
// 参数:
//   - dataShards: 数据分片数量
//   - parityShards: 校验分片数量
//   - opts: 可选的配置选项
//
// 返回值:
//   - Encoder: 创建的编码器实例
//   - error: 创建过程中的错误
//
// 说明:
//   - 总分片数量(数据分片+校验分片)最大为65536
//   - 当总分片数大于256时有以下限制:
//   - 分片大小必须是64的倍数
//   - 不支持Join/Split/Update/EncodeIdx方法
//   - 如果没有提供选项,将使用默认配置
func New(dataShards, parityShards int, opts ...Option) (Encoder, error) {
	// 使用默认选项
	o := defaultOptions
	// 应用传入的选项
	for _, opt := range opts {
		opt(&o)
	}

	// 计算总分片数
	totShards := dataShards + parityShards

	// 根据配置选择不同的编码器实现
	switch {
	case o.withLeopard == leopardGF16 && parityShards > 0 || totShards > 256:
		return newFF16(dataShards, parityShards, o)
	case o.withLeopard == leopardAlways && parityShards > 0:
		return newFF8(dataShards, parityShards, o)
	}

	// 检查总分片数是否超过限制
	if totShards > 256 {
		return nil, ErrMaxShardNum
	}

	// 创建Reed-Solomon编码器实例
	r := reedSolomon{
		dataShards:   dataShards,
		parityShards: parityShards,
		totalShards:  dataShards + parityShards,
		o:            o,
	}

	// 验证分片数量参数
	if dataShards <= 0 || parityShards < 0 {
		return nil, ErrInvShardNum
	}

	// 如果没有校验分片,直接返回
	if parityShards == 0 {
		return &r, nil
	}

	// 根据不同配置构建编码矩阵
	var err error
	switch {
	case r.o.customMatrix != nil:
		// 使用自定义矩阵
		if len(r.o.customMatrix) < parityShards {
			return nil, errors.New("编码矩阵必须至少包含校验分片数量的行")
		}
		r.m = make([][]byte, r.totalShards)
		// 构建单位矩阵部分
		for i := 0; i < dataShards; i++ {
			r.m[i] = make([]byte, dataShards)
			r.m[i][i] = 1
		}
		// 复制自定义矩阵部分
		for k, row := range r.o.customMatrix {
			if len(row) < dataShards {
				return nil, errors.New("编码矩阵必须至少包含数据分片数量的列")
			}
			r.m[dataShards+k] = make([]byte, dataShards)
			copy(r.m[dataShards+k], row)
		}
	case r.o.fastOneParity && parityShards == 1:
		// 使用XOR矩阵(单个校验分片)
		r.m, err = buildXorMatrix(dataShards, r.totalShards)
	case r.o.useCauchy:
		// 使用柯西矩阵
		r.m, err = buildMatrixCauchy(dataShards, r.totalShards)
	case r.o.usePAR1Matrix:
		// 使用PAR1矩阵
		r.m, err = buildMatrixPAR1(dataShards, r.totalShards)
	case r.o.useJerasureMatrix:
		// 使用Jerasure矩阵
		r.m, err = buildMatrixJerasure(dataShards, r.totalShards)
	default:
		// 使用默认范德蒙德矩阵
		r.m, err = buildMatrix(dataShards, r.totalShards)
	}
	if err != nil {
		return nil, err
	}

	// 计算每轮处理的数据量,基于L2缓存大小
	r.o.perRound = cpuid.CPU.Cache.L2
	if r.o.perRound < 128<<10 {
		r.o.perRound = 128 << 10
	}

	// 检查是否可以使用代码生成
	_, _, useCodeGen := r.hasCodeGen(codeGenMinSize, codeGenMaxInputs, codeGenMaxOutputs)

	// 计算分片处理的分割数
	divide := parityShards + 1
	if codeGen && useCodeGen && (dataShards > codeGenMaxInputs || parityShards > codeGenMaxOutputs) {
		// 如果输入较多,基于L1缓存计算
		r.o.perRound = cpuid.CPU.Cache.L1D
		if r.o.perRound < 32<<10 {
			r.o.perRound = 32 << 10
		}
		divide = 0
		if dataShards > codeGenMaxInputs {
			divide += codeGenMaxInputs
		} else {
			divide += dataShards
		}
		if parityShards > codeGenMaxInputs {
			divide += codeGenMaxOutputs
		} else {
			divide += parityShards
		}
	}

	// 调整多线程处理时的缓存使用
	if cpuid.CPU.ThreadsPerCore > 1 && r.o.maxGoroutines > cpuid.CPU.PhysicalCores {
		r.o.perRound /= cpuid.CPU.ThreadsPerCore
	}

	// 计算每轮处理的最终大小
	r.o.perRound = r.o.perRound / divide
	// 对齐到64字节
	r.o.perRound = ((r.o.perRound + 63) / 64) * 64

	// 最小值检查
	if r.o.perRound < 1<<10 {
		r.o.perRound = 1 << 10
	}

	// 设置最小分割大小
	if r.o.minSplitSize <= 0 {
		cacheSize := cpuid.CPU.Cache.L1D
		if cacheSize <= 0 {
			cacheSize = 32 << 10
		}

		r.o.minSplitSize = cacheSize / (parityShards + 1)
		if r.o.minSplitSize < 1024 {
			r.o.minSplitSize = 1024
		}
	}

	// 配置goroutine数量
	if r.o.shardSize > 0 {
		p := runtime.GOMAXPROCS(0)
		if p == 1 || r.o.shardSize <= r.o.minSplitSize*2 {
			r.o.maxGoroutines = 1
		} else {
			g := r.o.shardSize / r.o.perRound

			if g < p*2 && r.o.perRound > r.o.minSplitSize*2 {
				g = p * 2
				r.o.perRound /= 2
			}

			g += p - 1
			g -= g % p

			r.o.maxGoroutines = g
		}
	}

	// 限制使用代码生成时的goroutine数量
	if useCodeGen && r.o.maxGoroutines > codeGenMaxGoroutines {
		r.o.maxGoroutines = codeGenMaxGoroutines
	}

	// 限制使用GFNI时的goroutine数量
	if _, _, useGFNI := r.canGFNI(codeGenMinSize, codeGenMaxInputs, codeGenMaxOutputs); useGFNI && r.o.maxGoroutines > gfniCodeGenMaxGoroutines {
		r.o.maxGoroutines = gfniCodeGenMaxGoroutines
	}

	// 初始化矩阵求逆缓存
	if r.o.inversionCache {
		r.tree = newInversionTree(dataShards, parityShards)
	}

	// 提取校验矩阵
	r.parity = make([][]byte, parityShards)
	for i := range r.parity {
		r.parity[i] = r.m[dataShards+i]
	}

	// 初始化临时缓冲区池
	if codeGen {
		sz := r.dataShards * r.parityShards * 2 * 32
		r.mPool.New = func() interface{} {
			return AllocAligned(1, sz)[0]
		}
		r.mPoolSz = sz
	}
	return &r, err
}

// getTmpSlice 从临时缓冲区池中获取一个字节切片
// 返回值:
//   - []byte: 从池中获取的字节切片
func (r *reedSolomon) getTmpSlice() []byte {
	// 从池中获取一个对象并转换为字节切片
	return r.mPool.Get().([]byte)
}

// putTmpSlice 将使用完的字节切片放回临时缓冲区池中
// 参数:
//   - b: 要放回池中的字节切片
func (r *reedSolomon) putTmpSlice(b []byte) {
	// 检查切片是否有效且容量足够
	if b != nil && cap(b) >= r.mPoolSz {
		// 将切片截断到指定大小并放回池中
		r.mPool.Put(b[:r.mPoolSz])
		return
	}
	if false {
		// 完整性检查:验证返回的临时切片大小是否正确
		panic(fmt.Sprintf("got short tmp returned, want %d, got %d", r.mPoolSz, cap(b)))
	}
}

// ErrTooFewShards 在以下情况下返回:
// - 传入Encode/Verify/Reconstruct/Update的分片数量不足
// - 在Reconstruct中,可用分片数量不足以重建丢失的数据
var ErrTooFewShards = errors.New("too few shards given")

// Encode 对一组数据分片进行编码,生成校验分片
// 参数:
//   - shards: 包含数据分片和校验分片的数组,校验分片紧跟在数据分片之后
//     数据分片数量必须与创建编码器时指定的数量相同
//     每个分片都是字节数组,且必须大小相同
//
// 返回值:
//   - error: 编码过程中的错误信息
//
// 说明:
//   - 校验分片会被覆盖写入
//   - 数据分片保持不变
//   - 如果分片数量不匹配或分片大小不一致,将返回错误
func (r *reedSolomon) Encode(shards [][]byte) error {
	// 检查分片总数是否正确
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	// 验证所有分片的有效性
	err := checkShards(shards, false)
	if err != nil {
		return err
	}

	// 获取校验分片的切片
	output := shards[r.dataShards:]

	// 对数据分片进行编码,生成校验分片
	r.codeSomeShards(r.parity, shards[0:r.dataShards], output[:r.parityShards], len(shards[0]))
	return nil
}

// EncodeIdx 为单个数据分片添加校验分片
// 参数:
//   - dataShard: 要编码的数据分片
//   - idx: 数据分片的索引位置
//   - parity: 校验分片数组,必须预先初始化为0
//
// 返回值:
//   - error: 编码过程中的错误信息
//
// 说明:
//   - 校验分片必须在首次调用前初始化为0
//   - 每个数据分片只能传入一次,不会检查重复传入
//   - 校验分片会被更新,数据分片保持不变
func (r *reedSolomon) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	// 检查校验分片数量是否正确
	if len(parity) != r.parityShards {
		return ErrTooFewShards
	}
	// 如果没有校验分片,直接返回
	if len(parity) == 0 {
		return nil
	}
	// 检查数据分片索引是否有效
	if idx < 0 || idx >= r.dataShards {
		return ErrInvShardNum
	}
	// 验证校验分片的有效性
	err := checkShards(parity, false)
	if err != nil {
		return err
	}
	// 检查数据分片和校验分片的大小是否一致
	if len(parity[0]) != len(dataShard) {
		return ErrShardSize
	}

	// 使用硬件加速(AVX512/GFNI)进行编码
	if codeGen && len(dataShard) >= r.o.perRound && len(parity) >= codeGenMinShards && (pshufb || r.o.useAvx512GFNI || r.o.useAvxGNFI) {
		// 创建编码矩阵切片
		m := make([][]byte, r.parityShards)
		for iRow := range m {
			m[iRow] = r.parity[iRow][idx : idx+1]
		}
		// 根据CPU特性选择不同的编码实现
		if r.o.useAvx512GFNI || r.o.useAvxGNFI {
			r.codeSomeShardsGFNI(m, [][]byte{dataShard}, parity, len(dataShard), false, nil, nil)
		} else {
			r.codeSomeShardsAVXP(m, [][]byte{dataShard}, parity, len(dataShard), false, nil, nil)
		}
		return nil
	}

	// 不使用并发处理的情况
	// 初始化处理范围
	start, end := 0, r.o.perRound
	if end > len(dataShard) {
		end = len(dataShard)
	}

	// 分块处理数据
	for start < len(dataShard) {
		in := dataShard[start:end]
		// 计算每个校验分片
		for iRow := 0; iRow < r.parityShards; iRow++ {
			galMulSliceXor(r.parity[iRow][idx], in, parity[iRow][start:end], &r.o)
		}
		// 更新处理范围
		start = end
		end += r.o.perRound
		if end > len(dataShard) {
			end = len(dataShard)
		}
	}
	return nil
}

// ErrInvalidInput 当Update函数的输入参数无效时返回此错误
var ErrInvalidInput = errors.New("invalid input")

// Update 更新校验分片
// 参数:
//   - shards: 包含数据分片和校验分片的完整分片数组
//   - newDatashards: 新的数据分片数组
//
// 返回值:
//   - error: 如果输入参数无效或更新过程中出错则返回相应错误,否则返回nil
func (r *reedSolomon) Update(shards [][]byte, newDatashards [][]byte) error {
	// 检查分片总数是否正确
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	// 检查新数据分片数量是否正确
	if len(newDatashards) != r.dataShards {
		return ErrTooFewShards
	}

	// 验证原始分片的有效性
	err := checkShards(shards, true)
	if err != nil {
		return err
	}

	// 验证新数据分片的有效性
	err = checkShards(newDatashards, true)
	if err != nil {
		return err
	}

	// 检查新旧数据分片的一致性
	for i := range newDatashards {
		if newDatashards[i] != nil && shards[i] == nil {
			return ErrInvalidInput
		}
	}
	// 检查校验分片是否完整
	for _, p := range shards[r.dataShards:] {
		if p == nil {
			return ErrInvalidInput
		}
	}

	// 获取分片大小
	shardSize := shardSize(shards)

	// 获取校验分片切片
	output := shards[r.dataShards:]

	// 更新校验分片
	r.updateParityShards(r.parity, shards[0:r.dataShards], newDatashards[0:r.dataShards], output, r.parityShards, shardSize)
	return nil
}

// updateParityShards 更新校验分片的具体实现
// 参数:
//   - matrixRows: 编码矩阵行
//   - oldinputs: 原始数据分片
//   - newinputs: 新数据分片
//   - outputs: 输出的校验分片
//   - outputCount: 校验分片数量
//   - byteCount: 分片字节数
func (r *reedSolomon) updateParityShards(matrixRows, oldinputs, newinputs, outputs [][]byte, outputCount, byteCount int) {
	// 如果没有输出分片,直接返回
	if len(outputs) == 0 {
		return
	}

	// 如果满足并发处理条件,使用并发方式更新
	if r.o.maxGoroutines > 1 && byteCount > r.o.minSplitSize {
		r.updateParityShardsP(matrixRows, oldinputs, newinputs, outputs, outputCount, byteCount)
		return
	}

	// 顺序处理每个数据分片
	for c := 0; c < r.dataShards; c++ {
		in := newinputs[c]
		if in == nil {
			continue
		}
		oldin := oldinputs[c]
		// 计算新旧数据分片的异或值
		sliceXor(in, oldin, &r.o)
		// 更新每个校验分片
		for iRow := 0; iRow < outputCount; iRow++ {
			galMulSliceXor(matrixRows[iRow][c], oldin, outputs[iRow], &r.o)
		}
	}
}

// updateParityShardsP 并发更新校验分片
// 参数:
//   - matrixRows: 编码矩阵行
//   - oldinputs: 原始数据分片
//   - newinputs: 新数据分片
//   - outputs: 输出的校验分片
//   - outputCount: 校验分片数量
//   - byteCount: 分片字节数
func (r *reedSolomon) updateParityShardsP(matrixRows, oldinputs, newinputs, outputs [][]byte, outputCount, byteCount int) {
	// 创建等待组用于同步goroutine
	var wg sync.WaitGroup

	// 计算每个goroutine处理的数据大小
	do := byteCount / r.o.maxGoroutines
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}

	// 按照数据块大小分割并发处理
	start := 0
	for start < byteCount {
		// 处理最后一块可能不足do大小的情况
		if start+do > byteCount {
			do = byteCount - start
		}

		// 启动goroutine处理当前数据块
		wg.Add(1)
		go func(start, stop int) {
			// 处理每个数据分片
			for c := 0; c < r.dataShards; c++ {
				in := newinputs[c]
				if in == nil {
					continue
				}
				oldin := oldinputs[c]
				// 计算新旧数据分片指定范围的异或值
				sliceXor(in[start:stop], oldin[start:stop], &r.o)
				// 更新每个校验分片
				for iRow := 0; iRow < outputCount; iRow++ {
					galMulSliceXor(matrixRows[iRow][c], oldin[start:stop], outputs[iRow][start:stop], &r.o)
				}
			}
			wg.Done()
		}(start, start+do)
		start += do
	}

	// 等待所有goroutine完成
	wg.Wait()
}

// Verify 验证校验分片是否包含正确的数据
// 参数:
//   - shards: 包含数据分片和校验分片的切片数组
//
// 返回值:
//   - bool: 验证是否通过
//   - error: 错误信息
func (r *reedSolomon) Verify(shards [][]byte) (bool, error) {
	// 检查分片数量是否正确
	if len(shards) != r.totalShards {
		return false, ErrTooFewShards
	}

	// 验证分片的有效性
	err := checkShards(shards, false)
	if err != nil {
		return false, err
	}

	// 获取需要检查的校验分片
	toCheck := shards[r.dataShards:]

	// 执行校验并返回结果
	return r.checkSomeShards(r.parity, shards[:r.dataShards], toCheck[:r.parityShards], len(shards[0])), nil
}

// codeSomeShards 使用编码矩阵的部分行对输入分片进行编码生成输出分片
// 参数:
//   - matrixRows: 编码矩阵的行
//   - inputs: 输入分片数组
//   - outputs: 输出分片数组
//   - byteCount: 处理的字节数
func (r *reedSolomon) codeSomeShards(matrixRows, inputs, outputs [][]byte, byteCount int) {
	// 如果没有输出分片，直接返回
	if len(outputs) == 0 {
		return
	}

	// 如果数据量超过最小分割大小，使用并行处理
	if byteCount > r.o.minSplitSize {
		r.codeSomeShardsP(matrixRows, inputs, outputs, byteCount)
		return
	}

	// 初始化处理范围
	start, end := 0, r.o.perRound
	if end > len(inputs[0]) {
		end = len(inputs[0])
	}

	// 检查是否可以使用GFNI指令集优化
	if galMulGFNI, galMulGFNIXor, useGFNI := r.canGFNI(byteCount, len(inputs), len(outputs)); useGFNI {
		var gfni [codeGenMaxInputs * codeGenMaxOutputs]uint64
		m := genGFNIMatrix(matrixRows, len(inputs), 0, len(outputs), gfni[:])
		start += (*galMulGFNI)(m, inputs, outputs, 0, byteCount)
		end = len(inputs[0])
	} else if galMulGen, _, ok := r.hasCodeGen(byteCount, len(inputs), len(outputs)); ok {
		// 使用代码生成优化
		m := genCodeGenMatrix(matrixRows, len(inputs), 0, len(outputs), r.o.vectorLength, r.getTmpSlice())
		start += (*galMulGen)(m, inputs, outputs, 0, byteCount)
		r.putTmpSlice(m)
		end = len(inputs[0])
	} else if galMulGen, galMulGenXor, ok := r.hasCodeGen(byteCount, codeGenMaxInputs, codeGenMaxOutputs); len(inputs)+len(outputs) > codeGenMinShards && ok {
		// 使用分块处理的代码生成优化
		var gfni [codeGenMaxInputs * codeGenMaxOutputs]uint64
		end = len(inputs[0])
		inIdx := 0
		m := r.getTmpSlice()
		defer r.putTmpSlice(m)
		ins := inputs
		for len(ins) > 0 {
			inPer := ins
			if len(inPer) > codeGenMaxInputs {
				inPer = inPer[:codeGenMaxInputs]
			}
			outs := outputs
			outIdx := 0
			for len(outs) > 0 {
				outPer := outs
				if len(outPer) > codeGenMaxOutputs {
					outPer = outPer[:codeGenMaxOutputs]
				}
				if useGFNI {
					m := genGFNIMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), gfni[:])
					if inIdx == 0 {
						start = (*galMulGFNI)(m, inPer, outPer, 0, byteCount)
					} else {
						start = (*galMulGFNIXor)(m, inPer, outPer, 0, byteCount)
					}
				} else {
					m = genCodeGenMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), r.o.vectorLength, m)
					if inIdx == 0 {
						start = (*galMulGen)(m, inPer, outPer, 0, byteCount)
					} else {
						start = (*galMulGenXor)(m, inPer, outPer, 0, byteCount)
					}
				}
				outIdx += len(outPer)
				outs = outs[len(outPer):]
			}
			inIdx += len(inPer)
			ins = ins[len(inPer):]
		}
		if start >= end {
			return
		}
	}

	// 使用标准处理方式处理剩余数据
	for start < len(inputs[0]) {
		for c := 0; c < len(inputs); c++ {
			in := inputs[c][start:end]
			for iRow := 0; iRow < len(outputs); iRow++ {
				if c == 0 {
					galMulSlice(matrixRows[iRow][c], in, outputs[iRow][start:end], &r.o)
				} else {
					galMulSliceXor(matrixRows[iRow][c], in, outputs[iRow][start:end], &r.o)
				}
			}
		}
		start = end
		end += r.o.perRound
		if end > len(inputs[0]) {
			end = len(inputs[0])
		}
	}
}

// codeSomeShardsP 与codeSomeShards功能相同,但将工作负载分配到多个goroutine中并行处理
// 参数:
//   - matrixRows: 编码矩阵行
//   - inputs: 输入数据分片
//   - outputs: 输出校验分片
//   - byteCount: 处理的字节数
func (r *reedSolomon) codeSomeShardsP(matrixRows, inputs, outputs [][]byte, byteCount int) {
	var wg sync.WaitGroup
	gor := r.o.maxGoroutines // 获取最大goroutine数量

	var genMatrix []byte    // 生成矩阵
	var gfniMatrix []uint64 // GFNI矩阵

	// 检查是否可以使用代码生成和GFNI加速
	galMulGen, _, useCodeGen := r.hasCodeGen(byteCount, len(inputs), len(outputs))
	galMulGFNI, _, useGFNI := r.canGFNI(byteCount, len(inputs), len(outputs))

	// 根据不同条件选择处理方式
	if useGFNI { // 使用GFNI加速
		var tmp [codeGenMaxInputs * codeGenMaxOutputs]uint64
		gfniMatrix = genGFNIMatrix(matrixRows, len(inputs), 0, len(outputs), tmp[:])
	} else if useCodeGen { // 使用代码生成
		genMatrix = genCodeGenMatrix(matrixRows, len(inputs), 0, len(outputs), r.o.vectorLength, r.getTmpSlice())
		defer r.putTmpSlice(genMatrix)
	} else if galMulGFNI, galMulGFNIXor, useGFNI := r.canGFNI(byteCount/4, codeGenMaxInputs, codeGenMaxOutputs); useGFNI &&
		byteCount < 10<<20 && len(inputs)+len(outputs) > codeGenMinShards {
		// 对于10MB以下的数据,使用GFNI并行处理更快
		r.codeSomeShardsGFNI(matrixRows, inputs, outputs, byteCount, true, galMulGFNI, galMulGFNIXor)
		return
	} else if galMulGen, galMulGenXor, ok := r.hasCodeGen(byteCount/4, codeGenMaxInputs, codeGenMaxOutputs); ok &&
		byteCount < 10<<20 && len(inputs)+len(outputs) > codeGenMinShards {
		// 对于10MB以下的数据,使用AVX并行处理更快
		r.codeSomeShardsAVXP(matrixRows, inputs, outputs, byteCount, true, galMulGen, galMulGenXor)
		return
	}

	// 计算每个goroutine处理的数据大小
	do := byteCount / gor
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}

	// 定义执行函数
	exec := func(start, stop int) {
		// 对于较大数据块使用硬件加速
		if stop-start >= 64 {
			if useGFNI {
				start += (*galMulGFNI)(gfniMatrix, inputs, outputs, start, stop)
			} else if useCodeGen {
				start += (*galMulGen)(genMatrix, inputs, outputs, start, stop)
			}
		}

		// 处理剩余数据
		lstart, lstop := start, start+r.o.perRound
		if lstop > stop {
			lstop = stop
		}
		for lstart < stop {
			// 对每个输入分片进行处理
			for c := 0; c < len(inputs); c++ {
				in := inputs[c][lstart:lstop]
				// 计算每个输出分片
				for iRow := 0; iRow < len(outputs); iRow++ {
					if c == 0 {
						galMulSlice(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					} else {
						galMulSliceXor(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					}
				}
			}
			lstart = lstop
			lstop += r.o.perRound
			if lstop > stop {
				lstop = stop
			}
		}
		wg.Done()
	}

	// 如果只有一个goroutine,直接执行
	if gor <= 1 {
		wg.Add(1)
		exec(0, byteCount)
		return
	}

	// 将数据大小调整为64的倍数
	do = (do + 63) & (^63)
	start := 0
	// 启动多个goroutine并行处理数据
	for start < byteCount {
		if start+do > byteCount {
			do = byteCount - start
		}

		wg.Add(1)
		go exec(start, start+do)
		start += do
	}
	wg.Wait() // 等待所有goroutine完成
}

// codeSomeShardsAVXP 使用多个goroutine并行执行编码操作
// 参数:
//   - matrixRows: 编码矩阵的行
//   - inputs: 输入数据分片
//   - outputs: 输出数据分片
//   - byteCount: 需要处理的字节数
//   - clear: 是否在第一次写入时覆盖输出
//   - galMulGen: Galois域乘法生成函数指针
//   - galMulGenXor: Galois域乘法异或函数指针
func (r *reedSolomon) codeSomeShardsAVXP(matrixRows, inputs, outputs [][]byte, byteCount int, clear bool, galMulGen, galMulGenXor *func(matrix []byte, in [][]byte, out [][]byte, start int, stop int) int) {
	// 声明等待组用于同步goroutine
	var wg sync.WaitGroup
	// 获取最大goroutine数量
	gor := r.o.maxGoroutines

	// 定义状态结构体,用于存储编码计划
	type state struct {
		input  [][]byte // 输入分片
		output [][]byte // 输出分片
		m      []byte   // 编码矩阵
		first  bool     // 是否为第一次写入
	}
	// 创建编码计划切片
	plan := make([]state, 0, ((len(inputs)+codeGenMaxInputs-1)/codeGenMaxInputs)*((len(outputs)+codeGenMaxOutputs-1)/codeGenMaxOutputs))

	// 获取临时缓冲区
	tmp := r.getTmpSlice()
	defer r.putTmpSlice(tmp)

	// 根据输入输出数量选择不同的处理策略
	// 将较小的数据负载放在内循环中
	if len(inputs) > len(outputs) {
		// 输入分片数量较多时的处理逻辑
		inIdx := 0
		ins := inputs
		for len(ins) > 0 {
			// 获取当前处理的输入分片
			inPer := ins
			if len(inPer) > codeGenMaxInputs {
				inPer = inPer[:codeGenMaxInputs]
			}
			outs := outputs
			outIdx := 0
			for len(outs) > 0 {
				// 获取当前处理的输出分片
				outPer := outs
				if len(outPer) > codeGenMaxOutputs {
					outPer = outPer[:codeGenMaxOutputs]
				}
				// 生成局部编码矩阵
				m := genCodeGenMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), r.o.vectorLength, tmp)
				tmp = tmp[len(m):]
				// 将当前状态添加到计划中
				plan = append(plan, state{
					input:  inPer,
					output: outPer,
					m:      m,
					first:  inIdx == 0 && clear,
				})
				outIdx += len(outPer)
				outs = outs[len(outPer):]
			}
			inIdx += len(inPer)
			ins = ins[len(inPer):]
		}
	} else {
		// 输出分片数量较多时的处理逻辑
		outs := outputs
		outIdx := 0
		for len(outs) > 0 {
			// 获取当前处理的输出分片
			outPer := outs
			if len(outPer) > codeGenMaxOutputs {
				outPer = outPer[:codeGenMaxOutputs]
			}

			inIdx := 0
			ins := inputs
			for len(ins) > 0 {
				// 获取当前处理的输入分片
				inPer := ins
				if len(inPer) > codeGenMaxInputs {
					inPer = inPer[:codeGenMaxInputs]
				}
				// 生成局部编码矩阵
				m := genCodeGenMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), r.o.vectorLength, tmp)
				tmp = tmp[len(m):]
				// 将当前状态添加到计划中
				plan = append(plan, state{
					input:  inPer,
					output: outPer,
					m:      m,
					first:  inIdx == 0 && clear,
				})
				inIdx += len(inPer)
				ins = ins[len(inPer):]
			}
			outIdx += len(outPer)
			outs = outs[len(outPer):]
		}
	}

	// 计算每个goroutine处理的数据大小
	do := byteCount / gor
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}

	// 定义执行函数
	exec := func(start, stop int) {
		defer wg.Done()
		// 初始化处理范围
		lstart, lstop := start, start+r.o.perRound
		if lstop > stop {
			lstop = stop
		}
		for lstart < stop {
			// 使用优化的编码函数处理数据
			if galMulGen != nil && galMulGenXor != nil && lstop-lstart >= minCodeGenSize {
				// 执行编码计划
				var n int
				for _, p := range plan {
					if p.first {
						n = (*galMulGen)(p.m, p.input, p.output, lstart, lstop)
					} else {
						n = (*galMulGenXor)(p.m, p.input, p.output, lstart, lstop)
					}
				}
				lstart += n
				if lstart == lstop {
					lstop += r.o.perRound
					if lstop > stop {
						lstop = stop
					}
					continue
				}
			}

			// 使用标准编码方式处理数据
			for c := range inputs {
				in := inputs[c][lstart:lstop]
				for iRow := 0; iRow < len(outputs); iRow++ {
					if c == 0 && clear {
						galMulSlice(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					} else {
						galMulSliceXor(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					}
				}
			}
			lstart = lstop
			lstop += r.o.perRound
			if lstop > stop {
				lstop = stop
			}
		}
	}

	// 单goroutine处理
	if gor == 1 {
		wg.Add(1)
		exec(0, byteCount)
		return
	}

	// 将数据大小调整为64的倍数
	do = (do + 63) & (^63)
	start := 0
	// 启动多个goroutine并行处理数据
	for start < byteCount {
		if start+do > byteCount {
			do = byteCount - start
		}

		wg.Add(1)
		go exec(start, start+do)
		start += do
	}
	// 等待所有goroutine完成
	wg.Wait()
}

// codeSomeShardsGFNI 使用GFNI指令集并行处理数据编码
// 参数:
//   - matrixRows: 编码矩阵的行
//   - inputs: 输入数据分片
//   - outputs: 输出数据分片
//   - byteCount: 需要处理的字节数
//   - clear: 是否清除输出分片中的原有数据
//   - galMulGFNI: GFNI乘法运算函数指针
//   - galMulGFNIXor: GFNI异或运算函数指针
//
// 说明:
//   - 使用GFNI指令集加速编码过程
//   - 支持并行处理以提高性能
func (r *reedSolomon) codeSomeShardsGFNI(matrixRows, inputs, outputs [][]byte, byteCount int, clear bool, galMulGFNI, galMulGFNIXor *func(matrix []uint64, in, out [][]byte, start, stop int) int) {
	// 声明等待组用于同步goroutine
	var wg sync.WaitGroup
	// 获取最大goroutine数量
	gor := r.o.maxGoroutines

	// 定义状态结构体,用于存储计算所需的数据
	type state struct {
		input  [][]byte // 输入数据分片
		output [][]byte // 输出数据分片
		m      []uint64 // GFNI矩阵
		first  bool     // 是否为第一次写入
	}
	// 创建计划切片,预分配容量以提高性能
	plan := make([]state, 0, ((len(inputs)+codeGenMaxInputs-1)/codeGenMaxInputs)*((len(outputs)+codeGenMaxOutputs-1)/codeGenMaxOutputs))

	// 根据输入输出数量选择不同的处理策略
	// 将较小的数据量放在内循环中以提高性能
	if len(inputs) > len(outputs) {
		// 输入数量较多时,优先处理输入
		inIdx := 0
		ins := inputs
		for len(ins) > 0 {
			// 按最大输入数量分割
			inPer := ins
			if len(inPer) > codeGenMaxInputs {
				inPer = inPer[:codeGenMaxInputs]
			}
			outs := outputs
			outIdx := 0
			for len(outs) > 0 {
				// 按最大输出数量分割
				outPer := outs
				if len(outPer) > codeGenMaxOutputs {
					outPer = outPer[:codeGenMaxOutputs]
				}
				// 生成本地GFNI矩阵
				m := genGFNIMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), make([]uint64, len(inPer)*len(outPer)))
				// 将当前状态添加到计划中
				plan = append(plan, state{
					input:  inPer,
					output: outPer,
					m:      m,
					first:  inIdx == 0 && clear,
				})
				outIdx += len(outPer)
				outs = outs[len(outPer):]
			}
			inIdx += len(inPer)
			ins = ins[len(inPer):]
		}
	} else {
		// 输出数量较多时,优先处理输出
		outs := outputs
		outIdx := 0
		for len(outs) > 0 {
			// 按最大输出数量分割
			outPer := outs
			if len(outPer) > codeGenMaxOutputs {
				outPer = outPer[:codeGenMaxOutputs]
			}

			inIdx := 0
			ins := inputs
			for len(ins) > 0 {
				// 按最大输入数量分割
				inPer := ins
				if len(inPer) > codeGenMaxInputs {
					inPer = inPer[:codeGenMaxInputs]
				}
				// 生成本地GFNI矩阵
				m := genGFNIMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), make([]uint64, len(inPer)*len(outPer)))
				// 将当前状态添加到计划中
				plan = append(plan, state{
					input:  inPer,
					output: outPer,
					m:      m,
					first:  inIdx == 0 && clear,
				})
				inIdx += len(inPer)
				ins = ins[len(inPer):]
			}
			outIdx += len(outPer)
			outs = outs[len(outPer):]
		}
	}

	// 计算每个goroutine处理的数据大小
	do := byteCount / gor
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}

	// 定义数据处理函数
	exec := func(start, stop int) {
		defer wg.Done()
		// 初始化处理区间
		lstart, lstop := start, start+r.o.perRound
		if lstop > stop {
			lstop = stop
		}
		// 循环处理数据
		for lstart < stop {
			// 使用GFNI指令集处理数据
			if galMulGFNI != nil && galMulGFNIXor != nil && lstop-lstart >= minCodeGenSize {
				var n int
				// 执行计划中的每个状态
				for _, p := range plan {
					if p.first {
						n = (*galMulGFNI)(p.m, p.input, p.output, lstart, lstop)
					} else {
						n = (*galMulGFNIXor)(p.m, p.input, p.output, lstart, lstop)
					}
				}
				lstart += n
				if lstart == lstop {
					lstop += r.o.perRound
					if lstop > stop {
						lstop = stop
					}
					continue
				}
			}

			// 使用标准方式处理数据
			for c := range inputs {
				in := inputs[c][lstart:lstop]
				for iRow := 0; iRow < len(outputs); iRow++ {
					if c == 0 && clear {
						galMulSlice(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					} else {
						galMulSliceXor(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					}
				}
			}
			lstart = lstop
			lstop += r.o.perRound
			if lstop > stop {
				lstop = stop
			}
		}
	}

	// 单goroutine处理
	if gor == 1 {
		wg.Add(1)
		exec(0, byteCount)
		return
	}

	// 将数据大小调整为64的倍数
	do = (do + 63) & (^63)
	start := 0
	// 启动多个goroutine并行处理数据
	for start < byteCount {
		if start+do > byteCount {
			do = byteCount - start
		}

		wg.Add(1)
		go exec(start, start+do)
		start += do
	}
	// 等待所有goroutine完成
	wg.Wait()
}

// checkSomeShards 检查部分分片数据是否一致
// 该方法与 codeSomeShards 基本相同，但会在发现差异时立即返回
// 参数:
//   - matrixRows: 编码矩阵的行
//   - inputs: 输入的数据分片
//   - toCheck: 需要检查的数据分片
//   - byteCount: 需要处理的字节数
//
// 返回值:
//   - bool: 所有分片数据一致返回true,否则返回false
func (r *reedSolomon) checkSomeShards(matrixRows, inputs, toCheck [][]byte, byteCount int) bool {
	// 如果没有需要检查的分片,直接返回true
	if len(toCheck) == 0 {
		return true
	}

	// 分配对齐的内存空间用于存储计算结果
	outputs := AllocAligned(len(toCheck), byteCount)
	// 使用编码矩阵计算校验数据
	r.codeSomeShards(matrixRows, inputs, outputs, byteCount)

	// 逐个比较计算结果与待检查数据是否一致
	for i, calc := range outputs {
		if !bytes.Equal(calc, toCheck[i]) {
			// 如果发现不一致,立即返回false
			return false
		}
	}
	// 所有数据都一致,返回true
	return true
}

// ErrShardNoData 在以下情况下返回:
// - 没有分片数据
// - 所有分片的长度都为0
var ErrShardNoData = errors.New("没有分片数据")

// ErrShardSize 在分片长度不一致时返回
var ErrShardSize = errors.New("分片大小不一致")

// ErrInvalidShardSize 在分片长度不满足要求时返回,
// 通常要求分片长度是N的倍数
var ErrInvalidShardSize = errors.New("分片大小无效")

// checkShards 检查分片大小是否一致
// 参数:
//   - shards: 需要检查的分片数组
//   - nilok: 是否允许分片为空
//
// 返回值:
//   - error: 检查失败时返回相应错误,成功返回nil
//
// 说明:
//   - 检查所有分片大小是否相同或为0(如果允许)
//   - 如果所有分片大小都为0,返回ErrShardNoData错误
//   - 如果分片大小不一致且不允许为空,返回ErrShardSize错误
func checkShards(shards [][]byte, nilok bool) error {
	// 获取分片的标准大小
	size := shardSize(shards)
	// 如果所有分片大小都为0,返回错误
	if size == 0 {
		return ErrShardNoData
	}
	// 遍历检查每个分片的大小
	for _, shard := range shards {
		// 如果分片大小与标准大小不一致
		if len(shard) != size {
			// 如果分片不为空或不允许为空,返回错误
			if len(shard) != 0 || !nilok {
				return ErrShardSize
			}
		}
	}
	// 检查通过,返回nil
	return nil
}

// shardSize 获取分片的标准大小
// 参数:
//   - shards: 分片数组
//
// 返回值:
//   - int: 返回第一个非零分片的大小,如果所有分片都为0则返回0
func shardSize(shards [][]byte) int {
	// 遍历所有分片
	for _, shard := range shards {
		// 返回第一个非零分片的大小
		if len(shard) != 0 {
			return len(shard)
		}
	}
	// 所有分片都为0,返回0
	return 0
}

// Reconstruct 重建丢失的分片
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
//
// 说明:
//   - 给定一个分片数组,其中一些包含数据,填充那些没有数据的分片
//   - 数组长度必须等于分片总数
//   - 通过将分片设置为nil或零长度来表示分片丢失
//   - 如果分片为零长度但容量足够,将使用该内存,否则将分配新的[]byte
//   - 如果可用分片太少无法重建,将返回ErrTooFewShards错误
//   - 重建后的分片集是完整的,但未验证完整性,使用Verify函数检查数据集是否正确
func (r *reedSolomon) Reconstruct(shards [][]byte) error {
	// 调用内部重建方法,false表示重建所有分片,nil表示不指定特定分片
	return r.reconstruct(shards, false, nil)
}

// ReconstructData 仅重建丢失的数据分片
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
//
// 说明:
//   - 给定一个分片数组,其中一些包含数据,仅填充丢失的数据分片
//   - 数组长度必须等于分片总数
//   - 通过将分片设置为nil或零长度来表示分片丢失
//   - 如果分片为零长度但容量足够,将使用该内存,否则将分配新的[]byte
//   - 如果可用分片太少无法重建,将返回ErrTooFewShards错误
//   - 由于重建后的分片集可能包含缺失的校验分片,调用Verify函数可能会失败
func (r *reedSolomon) ReconstructData(shards [][]byte) error {
	// 调用内部重建方法,true表示只重建数据分片,nil表示不指定特定分片
	return r.reconstruct(shards, true, nil)
}

// ReconstructSome 仅重建指定的分片
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//   - required: 布尔数组,指示需要重建的分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
//
// 说明:
//   - 给定一个分片数组,其中一些包含数据,仅重建required参数中标记为true的分片
//   - required数组长度必须等于分片总数或数据分片数
//   - 如果长度等于数据分片数,将忽略校验分片的重建
//   - shards数组长度必须等于分片总数
//   - 通过将分片设置为nil或零长度来表示分片丢失
//   - 如果分片为零长度但容量足够,将使用该内存,否则将分配新的[]byte
//   - 如果可用分片太少无法重建,将返回ErrTooFewShards错误
func (r *reedSolomon) ReconstructSome(shards [][]byte, required []bool) error {
	// 如果required长度等于总分片数,重建指定的所有分片
	if len(required) == r.totalShards {
		return r.reconstruct(shards, false, required)
	}
	// 否则只重建指定的数据分片
	return r.reconstruct(shards, true, required)
}

// reconstruct 重建丢失的分片
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//   - dataOnly: 是否只重建数据分片
//   - required: 指定需要重建的分片,nil表示重建所有丢失分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
//
// 说明:
//   - shards数组长度必须等于分片总数
//   - 通过将分片设置为nil来表示分片丢失
//   - 如果可用分片太少无法重建,将返回ErrTooFewShards错误
func (r *reedSolomon) reconstruct(shards [][]byte, dataOnly bool, required []bool) error {
	// 检查分片数组长度和required参数是否合法
	if len(shards) != r.totalShards || required != nil && len(required) < r.dataShards {
		return ErrTooFewShards
	}
	// 检查分片数组参数是否合法
	err := checkShards(shards, true)
	if err != nil {
		return err
	}

	// 获取分片大小
	shardSize := shardSize(shards)

	// 快速检查:统计存在的分片数量
	numberPresent := 0   // 存在的总分片数
	dataPresent := 0     // 存在的数据分片数
	missingRequired := 0 // 需要重建的分片数
	for i := 0; i < r.totalShards; i++ {
		if len(shards[i]) != 0 {
			numberPresent++
			if i < r.dataShards {
				dataPresent++
			}
		} else if required != nil && required[i] {
			missingRequired++
		}
	}
	// 如果所有分片都存在,或只需要数据分片且数据分片完整,或所有需要重建的分片都存在,则无需重建
	if numberPresent == r.totalShards || dataOnly && dataPresent == r.dataShards ||
		required != nil && missingRequired == 0 {
		return nil
	}

	// 检查是否有足够的分片用于重建
	if numberPresent < r.dataShards {
		return ErrTooFewShards
	}

	// 创建子分片数组和索引数组
	subShards := make([][]byte, r.dataShards) // 用于重建的输入分片
	validIndices := make([]int, r.dataShards) // 有效分片的索引
	invalidIndices := make([]int, 0)          // 无效分片的索引
	subMatrixRow := 0
	// 收集有效分片和记录索引
	for matrixRow := 0; matrixRow < r.totalShards && subMatrixRow < r.dataShards; matrixRow++ {
		if len(shards[matrixRow]) != 0 {
			subShards[subMatrixRow] = shards[matrixRow]
			validIndices[subMatrixRow] = matrixRow
			subMatrixRow++
		} else {
			invalidIndices = append(invalidIndices, matrixRow)
		}
	}

	// 尝试从缓存树中获取反转矩阵
	dataDecodeMatrix := r.tree.GetInvertedMatrix(invalidIndices)

	// 如果缓存中没有反转矩阵,则构建并缓存
	if dataDecodeMatrix == nil {
		// 构建子矩阵
		subMatrix, _ := newMatrix(r.dataShards, r.dataShards)
		for subMatrixRow, validIndex := range validIndices {
			for c := 0; c < r.dataShards; c++ {
				subMatrix[subMatrixRow][c] = r.m[validIndex][c]
			}
		}
		// 计算反转矩阵
		dataDecodeMatrix, err = subMatrix.Invert()
		if err != nil {
			return err
		}

		// 将反转矩阵缓存到树中
		err = r.tree.InsertInvertedMatrix(invalidIndices, dataDecodeMatrix, r.totalShards)
		if err != nil {
			return err
		}
	}

	// 重建丢失的数据分片
	outputs := make([][]byte, r.parityShards)    // 输出缓冲区
	matrixRows := make([][]byte, r.parityShards) // 编码矩阵行
	outputCount := 0

	// 重建每个丢失的数据分片
	for iShard := 0; iShard < r.dataShards; iShard++ {
		if len(shards[iShard]) == 0 && (required == nil || required[iShard]) {
			// 分配或重用分片内存
			if cap(shards[iShard]) >= shardSize {
				shards[iShard] = shards[iShard][0:shardSize]
			} else {
				shards[iShard] = AllocAligned(1, shardSize)[0]
			}
			outputs[outputCount] = shards[iShard]
			matrixRows[outputCount] = dataDecodeMatrix[iShard]
			outputCount++
		}
	}
	// 执行数据分片重建
	r.codeSomeShards(matrixRows, subShards, outputs[:outputCount], shardSize)

	// 如果只需要重建数据分片,则返回
	if dataOnly {
		return nil
	}

	// 重建丢失的校验分片
	outputCount = 0
	// 重建每个丢失的校验分片
	for iShard := r.dataShards; iShard < r.totalShards; iShard++ {
		if len(shards[iShard]) == 0 && (required == nil || required[iShard]) {
			// 分配或重用分片内存
			if cap(shards[iShard]) >= shardSize {
				shards[iShard] = shards[iShard][0:shardSize]
			} else {
				shards[iShard] = AllocAligned(1, shardSize)[0]
			}
			outputs[outputCount] = shards[iShard]
			matrixRows[outputCount] = r.parity[iShard-r.dataShards]
			outputCount++
		}
	}
	// 执行校验分片重建
	r.codeSomeShards(matrixRows, shards[:r.dataShards], outputs[:outputCount], shardSize)
	return nil
}

// ErrShortData 当数据不足以填充所需分片数量时,Split()函数将返回此错误
var ErrShortData = errors.New("数据不足以填充请求的分片数量")

// Split 将数据切片分割成编码器指定数量的分片,并在必要时创建空的校验分片
//
// 参数:
//   - data: 需要分割的原始数据切片
//
// 返回值:
//   - [][]byte: 分割后的数据分片和校验分片
//   - error: 错误信息,如果数据长度为0则返回ErrShortData
//
// 说明:
// - 数据将被分割成大小相等的分片
// - 如果数据大小不能被分片数整除,最后一个分片将包含额外的零值填充
// - 如果提供的数据切片有额外容量,将被用于分配校验分片并被清零
// - 数据长度必须至少为1字节,否则返回ErrShortData
// - 除最后一个分片外,输入切片的数据不会被复制,因此后续不应修改输入数据
func (r *reedSolomon) Split(data []byte) ([][]byte, error) {
	// 检查输入数据是否为空
	if len(data) == 0 {
		return nil, ErrShortData
	}
	// 如果只有一个分片,直接返回原始数据
	if r.totalShards == 1 {
		return [][]byte{data}, nil
	}

	// 记录原始数据长度
	dataLen := len(data)
	// 计算每个数据分片的字节数
	perShard := (len(data) + r.dataShards - 1) / r.dataShards
	// 计算所需的总字节数
	needTotal := r.totalShards * perShard

	// 利用数据切片的额外容量
	if cap(data) > len(data) {
		if cap(data) > needTotal {
			data = data[:needTotal]
		} else {
			data = data[:cap(data)]
		}
		// 将额外容量部分清零
		clear := data[dataLen:]
		for i := range clear {
			clear[i] = 0
		}
	}

	// 仅在必要时分配内存
	var padding [][]byte
	if len(data) < needTotal {
		// 计算data切片中完整分片的最大数量
		fullShards := len(data) / perShard
		// 为剩余分片分配内存
		padding = AllocAligned(r.totalShards-fullShards, perShard)

		// 处理不完整的最后一个分片
		if dataLen > perShard*fullShards {
			// 复制部分分片数据
			copyFrom := data[perShard*fullShards : dataLen]
			for i := range padding {
				if len(copyFrom) == 0 {
					break
				}
				copyFrom = copyFrom[copy(padding[i], copyFrom):]
			}
		}
	}

	// 将数据分割成等长分片
	dst := make([][]byte, r.totalShards)
	i := 0
	// 分配完整的数据分片
	for ; i < len(dst) && len(data) >= perShard; i++ {
		dst[i] = data[:perShard:perShard]
		data = data[perShard:]
	}

	// 分配剩余的填充分片
	for j := 0; i+j < len(dst); j++ {
		dst[i+j] = padding[0]
		padding = padding[1:]
	}

	return dst, nil
}

// ErrReconstructRequired 在数据分片不完整需要重建时返回
// 当一个或多个必需的数据分片为空时,需要先进行重建才能成功合并分片
var ErrReconstructRequired = errors.New("需要重建,因为一个或多个必需的数据分片为空")

// Join 将数据分片合并并写入目标Writer
//
// 只处理数据分片,不包括校验分片
// 必须提供准确的输出大小
//
// 参数:
//   - dst: 目标Writer,用于写入合并后的数据
//   - shards: 要合并的数据分片切片
//   - outSize: 期望的输出数据大小
//
// 返回值:
//   - error: 如果分片数量不足返回ErrTooFewShards
//   - error: 如果总数据量小于outSize返回ErrShortData
//   - error: 如果存在空的必需数据分片返回ErrReconstructRequired
func (r *reedSolomon) Join(dst io.Writer, shards [][]byte, outSize int) error {
	// 检查分片数量是否足够
	if len(shards) < r.dataShards {
		return ErrTooFewShards
	}
	// 只取数据分片部分
	shards = shards[:r.dataShards]

	// 计算总数据量并检查数据完整性
	size := 0
	for _, shard := range shards {
		// 检查必需的数据分片是否存在
		if shard == nil {
			return ErrReconstructRequired
		}
		// 累加分片大小
		size += len(shard)

		// 如果已经达到所需大小则跳出
		if size >= outSize {
			break
		}
	}
	// 检查数据量是否足够
	if size < outSize {
		return ErrShortData
	}

	// 将数据写入目标Writer
	write := outSize
	for _, shard := range shards {
		// 如果剩余写入量小于当前分片大小
		if write < len(shard) {
			// 只写入需要的部分
			_, err := dst.Write(shard[:write])
			return err
		}
		// 写入完整分片
		n, err := dst.Write(shard)
		if err != nil {
			return err
		}
		// 更新剩余写入量
		write -= n
	}
	return nil
}
