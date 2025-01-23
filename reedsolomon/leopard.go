package reedsolomon

// 这是一个O(n*log n)复杂度的Reed-Solomon码实现,
// 移植自C++库 https://github.com/catid/leopard.
//
// 该实现基于以下论文:
//
// S.-J. Lin, T. Y. Al-Naffouri, Y. S. Han, and W.-H. Chung,
// "基于快速傅里叶变换的新型多项式基及其在Reed-Solomon纠删码中的应用"
// IEEE 信息理论汇刊, pp. 6284-6299, 2016年11月.

import (
	"bytes"
	"fmt"
	"io"
	"math/bits"
	"sync"
	"unsafe"

	"github.com/klauspost/cpuid/v2"
)

// leopardFF16 实现了一个基于有限域FF16的Reed-Solomon编码器
// 用于处理超过256个分片的情况,最多支持65536个分片
// 该实现基于快速傅里叶变换(FFT)算法,时间复杂度为O(n*log n)
type leopardFF16 struct {
	// dataShards 表示数据分片的数量
	// 这个值在初始化后不应被修改
	// 数据分片包含原始数据,每个分片大小相等
	dataShards int

	// parityShards 表示校验分片的数量
	// 这个值在初始化后不应被修改
	// 校验分片用于数据恢复,数量越多可以容忍更多分片丢失
	parityShards int

	// totalShards 表示所有分片的总数
	// 由 dataShards + parityShards 计算得出
	// 这个值在初始化后不应被修改
	totalShards int

	// workPool 是一个对象池,用于重用临时工作缓冲区
	// 这可以减少内存分配和GC压力
	workPool sync.Pool

	// o 包含编码器的配置选项
	// 如最大goroutine数量、向量指令等
	o options
}

// newFF16 创建一个新的FF16编码器实例,用于处理超过256个分片的情况
// 参数:
//   - dataShards: 数据分片数量
//   - parityShards: 校验分片数量
//   - opt: 编码器配置选项
//
// 返回值:
//   - *leopardFF16: 创建的编码器实例
//   - error: 错误信息,如果参数无效则返回错误
func newFF16(dataShards, parityShards int, opt options) (*leopardFF16, error) {
	// 初始化编码所需的常量表
	initConstants()

	// 验证分片数量是否有效
	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvShardNum
	}

	// 验证总分片数是否超过最大限制65536
	if dataShards+parityShards > 65536 {
		return nil, ErrMaxShardNum
	}

	// 创建并初始化leopardFF16实例
	r := &leopardFF16{
		dataShards:   dataShards,                // 设置数据分片数
		parityShards: parityShards,              // 设置校验分片数
		totalShards:  dataShards + parityShards, // 计算总分片数
		o:            opt,                       // 设置编码器选项
	}
	return r, nil // 返回创建的实例
}

// 确保leopardFF16实现了Extensions接口
var _ = Extensions(&leopardFF16{})

// ShardSizeMultiple 返回分片大小必须是其倍数的值
// 返回值:
//   - int: 分片大小倍数,固定为64
func (r *leopardFF16) ShardSizeMultiple() int {
	return 64 // 返回固定值64作为分片大小的倍数
}

// DataShards 返回数据分片的数量
// 返回值:
//   - int: 数据分片数量
func (r *leopardFF16) DataShards() int {
	return r.dataShards // 返回编码器中的数据分片数量
}

// ParityShards 返回校验分片的数量
// 返回值:
//   - int: 校验分片数量
func (r *leopardFF16) ParityShards() int {
	return r.parityShards // 返回编码器中的校验分片数量
}

// TotalShards 返回总分片数量
// 返回值:
//   - int: 总分片数量(数据分片+校验分片)
func (r *leopardFF16) TotalShards() int {
	return r.totalShards // 返回编码器中的总分片数量
}

// AllocAligned 分配对齐的内存空间用于存储分片数据
// 参数:
//   - each: 每个分片的大小
//
// 返回值:
//   - [][]byte: 分配的二维字节切片
func (r *leopardFF16) AllocAligned(each int) [][]byte {
	return AllocAligned(r.totalShards, each) // 调用全局AllocAligned函数分配内存
}

// ffe 是一个16位无符号整数类型,用于有限域运算
type ffe uint16

// 定义有限域运算相关的常量
const (
	bitwidth   = 16            // 位宽度
	order      = 1 << bitwidth // 有限域的阶(大小)
	modulus    = order - 1     // 模数
	polynomial = 0x1002D       // 生成多项式
)

// 定义FFT运算所需的查找表
var (
	fftSkew  *[modulus]ffe // FFT倾斜因子表
	logWalsh *[order]ffe   // Walsh变换的对数表
)

// 定义对数运算所需的查找表
var (
	logLUT *[order]ffe // 对数查找表
	expLUT *[order]ffe // 指数查找表
)

// mul16LUTs 存储x * y的部分积,偏移量为x + y * 65536
// 对相同y值的重复访问更快
var mul16LUTs *[order]mul16LUT

// mul16LUT 定义16位乘法查找表的结构
type mul16LUT struct {
	Lo [256]ffe // 低位乘积查找表
	Hi [256]ffe // 高位乘积查找表,需要与Lo异或得到最终结果
}

// multiply256LUT 存储AVX2指令集优化的查找表
var multiply256LUT *[order][8 * 16]byte

// Encode 对数据分片进行编码,生成校验分片
// 参数:
//   - shards: 包含数据分片和校验分片的二维字节切片
//
// 返回值:
//   - error: 编码失败时返回相应错误,成功返回nil
//
// 说明:
//   - 分片数组长度必须等于总分片数
//   - 所有分片大小必须相同且不为空
func (r *leopardFF16) Encode(shards [][]byte) error {
	// 检查分片数组长度是否正确
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	// 检查分片参数是否合法
	if err := checkShards(shards, false); err != nil {
		return err
	}
	// 调用内部编码方法
	return r.encode(shards)
}

// encode 执行实际的编码操作
// 参数:
//   - shards: 包含数据分片和校验分片的二维字节切片
//
// 返回值:
//   - error: 编码失败时返回相应错误,成功返回nil
//
// 说明:
//   - 使用FFT算法进行编码
//   - 分片大小必须是64的倍数
func (r *leopardFF16) encode(shards [][]byte) error {
	// 获取分片大小并检查是否为64的倍数
	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	// 计算大于等于校验分片数的最小2的幂
	m := ceilPow2(r.parityShards)
	// 从对象池获取工作空间
	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
	// 确保工作空间足够大
	if cap(work) >= m*2 {
		work = work[:m*2]
	} else {
		work = AllocAligned(m*2, shardSize)
	}
	// 初始化工作空间中的每个分片
	for i := range work {
		if cap(work[i]) < shardSize {
			work[i] = AllocAligned(1, shardSize)[0]
		} else {
			work[i] = work[i][:shardSize]
		}
	}
	// 使用完毕后将工作空间放回对象池
	defer r.workPool.Put(&work)

	// 计算实际使用的m值
	mtrunc := m
	if r.dataShards < mtrunc {
		mtrunc = r.dataShards
	}

	// 获取FFT倾斜因子表
	skewLUT := fftSkew[m-1:]

	// 对第一组数据分片执行IFFT变换
	sh := shards
	ifftDITEncoder(
		sh[:r.dataShards],
		mtrunc,
		work,
		nil, // 不需要异或输出
		m,
		skewLUT,
		&r.o,
	)

	// 计算最后一组数据分片的数量
	lastCount := r.dataShards % m
	if m >= r.dataShards {
		goto skip_body
	}

	// 处理完整的m个数据分片组
	for i := m; i+m <= r.dataShards; i += m {
		sh = sh[m:]
		skewLUT = skewLUT[m:]

		// 对每组数据执行IFFT并与work异或
		ifftDITEncoder(
			sh, // 数据源
			m,
			work[m:], // 临时工作空间
			work,     // 异或目标
			m,
			skewLUT,
			&r.o,
		)
	}

	// 处理最后一组不完整的数据分片
	if lastCount != 0 {
		sh = sh[m:]
		skewLUT = skewLUT[m:]

		// 对剩余数据执行IFFT并与work异或
		ifftDITEncoder(
			sh, // 数据源
			lastCount,
			work[m:], // 临时工作空间
			work,     // 异或目标
			m,
			skewLUT,
			&r.o,
		)
	}

skip_body:
	// 对work执行FFT变换生成校验分片
	fftDIT(work, r.parityShards, m, fftSkew[:], &r.o)

	// 将生成的校验分片复制到输出数组
	for i, w := range work[:r.parityShards] {
		sh := shards[i+r.dataShards]
		if cap(sh) >= shardSize {
			sh = append(sh[:0], w...)
		} else {
			sh = w
		}
		shards[i+r.dataShards] = sh
	}

	return nil
}

// EncodeIdx 对单个数据分片进行编码
// 参数:
//   - dataShard: 需要编码的数据分片
//   - idx: 数据分片的索引
//   - parity: 校验分片数组
//
// 返回值:
//   - error: 编码失败时返回相应错误,成功返回nil
//
// 说明:
//   - 该方法暂不支持,返回ErrNotSupported错误
func (r *leopardFF16) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	// 返回不支持错误
	return ErrNotSupported
}

// Join 将多个分片合并成一个完整的数据
// 参数:
//   - dst: 用于写入合并后数据的目标Writer
//   - shards: 需要合并的分片数组
//   - outSize: 期望输出的数据大小
//
// 返回值:
//   - error: 合并失败时返回相应错误,成功返回nil
//
// 说明:
//   - 检查是否有足够的分片和数据
//   - 按顺序将分片数据写入目标Writer
func (r *leopardFF16) Join(dst io.Writer, shards [][]byte, outSize int) error {
	// 检查分片数量是否足够
	if len(shards) < r.dataShards {
		return ErrTooFewShards
	}
	// 只使用数据分片部分
	shards = shards[:r.dataShards]

	// 计算可用数据总大小
	size := 0
	for _, shard := range shards {
		// 检查分片是否存在
		if shard == nil {
			return ErrReconstructRequired
		}
		// 累加分片大小
		size += len(shard)

		// 如果数据已足够,跳出循环
		if size >= outSize {
			break
		}
	}
	// 检查数据是否足够
	if size < outSize {
		return ErrShortData
	}

	// 将数据写入目标Writer
	write := outSize
	for _, shard := range shards {
		// 如果剩余需要写入的数据小于当前分片大小
		if write < len(shard) {
			// 只写入需要的部分
			_, err := dst.Write(shard[:write])
			return err
		}
		// 写入整个分片
		n, err := dst.Write(shard)
		if err != nil {
			return err
		}
		// 更新剩余需要写入的数据大小
		write -= n
	}
	return nil
}

// Update 更新已编码的分片
// 参数:
//   - shards: 已编码的分片数组
//   - newDatashards: 新的数据分片数组
//
// 返回值:
//   - error: 更新失败时返回相应错误,成功返回nil
//
// 说明:
//   - 该方法暂不支持,返回ErrNotSupported错误
func (r *leopardFF16) Update(shards [][]byte, newDatashards [][]byte) error {
	// 返回不支持错误
	return ErrNotSupported
}

// Split 将数据切片分割成编码器指定数量的分片,并在必要时创建空的校验分片
// 参数:
//   - data: 需要分割的原始数据切片
//
// 返回值:
//   - [][]byte: 分割后的数据分片和校验分片
//   - error: 错误信息,如果数据长度为0则返回ErrShortData
//
// 说明:
// - 数据将被分割成大小相等的分片,每个分片大小必须是64字节的倍数
// - 如果数据大小不能被分片数整除,最后一个分片将包含额外的零值填充
// - 如果提供的数据切片有额外容量,将被用于分配校验分片并被清零
func (r *leopardFF16) Split(data []byte) ([][]byte, error) {
	// 检查输入数据是否为空
	if len(data) == 0 {
		return nil, ErrShortData
	}

	// 如果只有一个分片且长度是64的倍数,直接返回原始数据
	if r.totalShards == 1 && len(data)&63 == 0 {
		return [][]byte{data}, nil
	}

	// 记录原始数据长度
	dataLen := len(data)
	// 计算每个数据分片的字节数,并向上取整到64字节的倍数
	perShard := (len(data) + r.dataShards - 1) / r.dataShards
	perShard = ((perShard + 63) / 64) * 64
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
	} else {
		// 如果数据足够,将多余部分清零
		zero := data[dataLen : r.totalShards*perShard]
		for i := range zero {
			zero[i] = 0
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

// ReconstructSome 仅重建指定的分片
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//   - required: 布尔数组,指示需要重建的分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
func (r *leopardFF16) ReconstructSome(shards [][]byte, required []bool) error {
	// 如果required长度等于总分片数,重建所有分片
	if len(required) == r.totalShards {
		return r.reconstruct(shards, true)
	}
	// 否则只重建数据分片
	return r.reconstruct(shards, false)
}

// Reconstruct 重建所有丢失的分片
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
func (r *leopardFF16) Reconstruct(shards [][]byte) error {
	// 调用内部重建方法,重建所有分片
	return r.reconstruct(shards, true)
}

// ReconstructData 仅重建丢失的数据分片
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
func (r *leopardFF16) ReconstructData(shards [][]byte) error {
	// 调用内部重建方法,只重建数据分片
	return r.reconstruct(shards, false)
}

// Verify 验证分片数据的完整性
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//
// 返回值:
//   - bool: 验证通过返回true,否则返回false
//   - error: 验证过程中出现错误时返回相应错误,成功返回nil
//
// 说明:
//   - 通过重新计算校验分片并与原有校验分片比较来验证数据完整性
//   - 如果分片数量不足或分片大小不一致将返回错误
//   - 如果校验分片与重新计算的结果不一致,返回false
func (r *leopardFF16) Verify(shards [][]byte) (bool, error) {
	// 检查分片数量是否正确
	if len(shards) != r.totalShards {
		return false, ErrTooFewShards
	}
	// 检查分片大小是否一致,不允许空分片
	if err := checkShards(shards, false); err != nil {
		return false, err
	}

	// 获取分片大小
	shardSize := len(shards[0])
	// 创建临时存储空间用于重新编码
	outputs := make([][]byte, r.totalShards)
	// 复制数据分片到临时存储
	copy(outputs, shards[:r.dataShards])
	// 为校验分片分配内存空间
	for i := r.dataShards; i < r.totalShards; i++ {
		outputs[i] = make([]byte, shardSize)
	}
	// 重新计算校验分片
	if err := r.Encode(outputs); err != nil {
		return false, err
	}

	// 比较重新计算的校验分片与原有校验分片
	for i := r.dataShards; i < r.totalShards; i++ {
		// 如果校验分片不一致,返回false
		if !bytes.Equal(outputs[i], shards[i]) {
			fmt.Printf("校验分片 %d 不一致:\n", i)
			fmt.Printf("期望值: %x\n", shards[i][:8])  // 只打印前8个字节
			fmt.Printf("实际值: %x\n", outputs[i][:8]) // 只打印前8个字节
			return false, nil
		}
	}
	// 所有校验分片都一致,返回true
	return true, nil
}

// reconstruct 重建丢失的分片数据
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//   - recoverAll: 是否恢复所有丢失的分片,包括校验分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
//
// 说明:
//   - 使用FFT算法重建丢失的分片
//   - 如果recoverAll为false,只恢复数据分片
//   - 如果recoverAll为true,同时恢复数据分片和校验分片
func (r *leopardFF16) reconstruct(shards [][]byte, recoverAll bool) error {
	// 检查分片数组长度是否正确
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	// 检查分片参数是否合法,允许空分片
	if err := checkShards(shards, true); err != nil {
		return err
	}

	// 统计现有分片数量
	numberPresent := 0 // 所有存在的分片数量
	dataPresent := 0   // 存在的数据分片数量
	// 遍历所有分片,统计存在的分片数量
	for i := 0; i < r.totalShards; i++ {
		if len(shards[i]) != 0 {
			numberPresent++
			if i < r.dataShards {
				dataPresent++
			}
		}
	}
	// 如果所有分片都存在,或者不需要恢复所有分片且数据分片完整,直接返回
	if numberPresent == r.totalShards || !recoverAll && dataPresent == r.dataShards {
		return nil
	}

	// 当丢失的校验分片少于1/4时使用位域优化
	useBits := r.totalShards-numberPresent <= r.parityShards/4

	// 检查是否有足够的分片用于重建
	if numberPresent < r.dataShards {
		return ErrTooFewShards
	}

	// 获取分片大小并检查是否为64的倍数
	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	// 计算大于等于校验分片数的最小2的幂
	m := ceilPow2(r.parityShards)
	// 计算大于等于m+数据分片数的最小2的幂
	n := ceilPow2(m + r.dataShards)

	// 是否启用错误位域优化
	const LEO_ERROR_BITFIELD_OPT = true

	// 初始化错误位置数组和位域
	var errorBits errorBitfield
	var errLocs [order]ffe
	// 标记丢失的校验分片位置
	for i := 0; i < r.parityShards; i++ {
		if len(shards[i+r.dataShards]) == 0 {
			errLocs[i] = 1
			if LEO_ERROR_BITFIELD_OPT && recoverAll {
				errorBits.set(i)
			}
		}
	}
	// 标记填充位置
	for i := r.parityShards; i < m; i++ {
		errLocs[i] = 1
		if LEO_ERROR_BITFIELD_OPT && recoverAll {
			errorBits.set(i)
		}
	}
	// 标记丢失的数据分片位置
	for i := 0; i < r.dataShards; i++ {
		if len(shards[i]) == 0 {
			errLocs[i+m] = 1
			if LEO_ERROR_BITFIELD_OPT {
				errorBits.set(i + m)
			}
		}
	}

	// 如果启用位域优化且满足条件,准备位域
	if LEO_ERROR_BITFIELD_OPT && useBits {
		errorBits.prepare()
	}

	// 计算错误定位多项式
	fwht(&errLocs, m+r.dataShards)

	// 对错误位置进行变换
	for i := 0; i < order; i++ {
		errLocs[i] = ffe((uint(errLocs[i]) * uint(logWalsh[i])) % modulus)
	}

	// 对错误位置进行Walsh-Hadamard变换
	fwht(&errLocs, order)

	// 从对象池获取或创建工作空间
	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
	if cap(work) >= n {
		work = work[:n]
	} else {
		work = make([][]byte, n)
	}
	// 确保工作空间中每个分片有足够的容量
	for i := range work {
		if cap(work[i]) < shardSize {
			work[i] = make([]byte, shardSize)
		} else {
			work[i] = work[i][:shardSize]
		}
	}
	// 使用完毕后将工作空间放回对象池
	defer r.workPool.Put(work)

	// 处理校验分片数据
	for i := 0; i < r.parityShards; i++ {
		if len(shards[i+r.dataShards]) != 0 {
			mulgf16(work[i], shards[i+r.dataShards], errLocs[i], &r.o)
		} else {
			memclr(work[i])
		}
	}
	// 清空填充部分
	for i := r.parityShards; i < m; i++ {
		memclr(work[i])
	}

	// 处理原始数据分片
	for i := 0; i < r.dataShards; i++ {
		if len(shards[i]) != 0 {
			mulgf16(work[m+i], shards[i], errLocs[m+i], &r.o)
		} else {
			memclr(work[m+i])
		}
	}
	// 清空剩余空间
	for i := m + r.dataShards; i < n; i++ {
		memclr(work[i])
	}

	// 对工作空间进行IFFT变换
	ifftDITDecoder(
		m+r.dataShards,
		work,
		n,
		fftSkew[:],
		&r.o,
	)

	// 计算形式导数
	for i := 1; i < n; i++ {
		width := ((i ^ (i - 1)) + 1) >> 1
		slicesXor(work[i-width:i], work[i:i+width], &r.o)
	}

	// 计算输出数量并进行FFT变换
	outputCount := m + r.dataShards

	// 根据是否使用位域优化选择不同的FFT实现
	if LEO_ERROR_BITFIELD_OPT && useBits {
		errorBits.fftDIT(work, outputCount, n, fftSkew[:], &r.o)
	} else {
		fftDIT(work, outputCount, n, fftSkew[:], &r.o)
	}

	// 恢复丢失的分片
	end := r.dataShards
	if recoverAll {
		end = r.totalShards
	}
	// 遍历需要恢复的分片
	for i := 0; i < end; i++ {
		if len(shards[i]) != 0 {
			continue
		}
		// 为丢失的分片分配内存
		if cap(shards[i]) >= shardSize {
			shards[i] = shards[i][:shardSize]
		} else {
			shards[i] = make([]byte, shardSize)
		}
		if i >= r.dataShards {
			// 恢复校验分片
			mulgf16(shards[i], work[i-r.dataShards], modulus-errLocs[i-r.dataShards], &r.o)
		} else {
			// 恢复数据分片
			mulgf16(shards[i], work[i+m], modulus-errLocs[i+m], &r.o)
		}
	}
	return nil
}

// ifftDITDecoder 执行解码器的快速傅里叶逆变换(IFFT)
// 参数:
//   - mtrunc: 需要处理的最大元素数量
//   - work: 工作空间,存储中间计算结果
//   - m: FFT点数,必须是2的幂
//   - skewLUT: 倾斜因子查找表
//   - o: 编码器选项
//
// 说明:
//   - 这是一个基本的解码器IFFT实现
//   - 使用按时间抽取(DIT)算法,每次展开2层
func ifftDITDecoder(mtrunc int, work [][]byte, m int, skewLUT []ffe, o *options) {
	// 初始化距离参数
	dist := 1  // 当前层的基本距离
	dist4 := 4 // 4倍距离,用于展开2层

	// 主循环:每次处理2层变换
	for dist4 <= m {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			// 计算当前组的结束位置
			iend := r + dist
			// 从查找表获取倾斜因子
			log_m01 := skewLUT[iend-1]        // 第一组倾斜因子
			log_m02 := skewLUT[iend+dist-1]   // 第二组倾斜因子
			log_m23 := skewLUT[iend+dist*2-1] // 第三组倾斜因子

			// 对每组dist个元素执行4点IFFT
			for i := r; i < iend; i++ {
				ifftDIT4(work[i:], dist, log_m01, log_m23, log_m02, o)
			}
		}
		// 更新距离参数
		dist = dist4 // 更新基本距离
		dist4 <<= 2  // 距离乘4
	}

	// 处理剩余的一层(如果存在)
	if dist < m {
		// 确保dist是m的一半
		if dist*2 != m {
			panic("内部错误:dist不是m的一半")
		}

		// 获取最后一层的倾斜因子
		log_m := skewLUT[dist-1]

		// 根据倾斜因子选择处理方式
		if log_m == modulus {
			// 如果倾斜因子等于模数,直接异或
			slicesXor(work[dist:2*dist], work[:dist], o)
		} else {
			// 否则执行2点IFFT
			for i := 0; i < dist; i++ {
				ifftDIT2(
					work[i],      // 第一个输入/输出
					work[i+dist], // 第二个输入/输出
					log_m,        // 倾斜因子
					o,            // 选项
				)
			}
		}
	}
}

// fftDIT 执行就地快速傅里叶变换,用于编码器和解码器
// 参数:
//   - work: 工作数组,存储输入和输出数据
//   - mtrunc: 实际数据长度
//   - m: 总长度(2的幂)
//   - skewLUT: 倾斜因子查找表
//   - o: 选项配置
func fftDIT(work [][]byte, mtrunc, m int, skewLUT []ffe, o *options) {
	// 按时间抽取: 每次展开2层
	dist4 := m     // 4倍距离初始值为m
	dist := m >> 2 // 基本距离为m/4

	// 主循环:处理所有完整的2层变换
	for dist != 0 {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist                  // 计算当前组的结束位置
			log_m01 := skewLUT[iend-1]        // 第一组倾斜因子
			log_m02 := skewLUT[iend+dist-1]   // 第二组倾斜因子
			log_m23 := skewLUT[iend+dist*2-1] // 第三组倾斜因子

			// 对每组dist个元素执行4点FFT
			for i := r; i < iend; i++ {
				fftDIT4(
					work[i:],
					dist,
					log_m01,
					log_m23,
					log_m02,
					o,
				)
			}
		}
		dist4 = dist // 更新4倍距离
		dist >>= 2   // 基本距离除以4
	}

	// 处理剩余的一层(如果存在)
	if dist4 == 2 {
		// 对每2个元素进行处理
		for r := 0; r < mtrunc; r += 2 {
			log_m := skewLUT[r+1-1] // 获取倾斜因子

			// 根据倾斜因子选择处理方式
			if log_m == modulus {
				sliceXor(work[r], work[r+1], o) // 如果倾斜因子等于模数,直接异或
			} else {
				fftDIT2(work[r], work[r+1], log_m, o) // 否则执行2点FFT
			}
		}
	}
}

// fftDIT4Ref 执行4路蝶形运算的参考实现
// 参数:
//   - work: 工作数组
//   - dist: 蝶形运算的步长
//   - log_m01,log_m23,log_m02: 倾斜因子
//   - o: 选项配置
func fftDIT4Ref(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	// 第一层变换:
	if log_m02 == modulus {
		// 如果倾斜因子等于模数,执行异或运算
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		// 否则执行2点FFT
		fftDIT2(work[0], work[dist*2], log_m02, o)
		fftDIT2(work[dist], work[dist*3], log_m02, o)
	}

	// 第二层变换:
	if log_m01 == modulus {
		sliceXor(work[0], work[dist], o) // 第一组:如果倾斜因子等于模数,执行异或
	} else {
		fftDIT2(work[0], work[dist], log_m01, o) // 否则执行2点FFT
	}

	if log_m23 == modulus {
		sliceXor(work[dist*2], work[dist*3], o) // 第二组:如果倾斜因子等于模数,执行异或
	} else {
		fftDIT2(work[dist*2], work[dist*3], log_m23, o) // 否则执行2点FFT
	}
}

// ifftDITEncoder 执行编码器的展开式IFFT变换
// 参数:
//   - data: 输入数据分片数组
//   - mtrunc: 有效数据分片数量
//   - work: 工作空间数组
//   - xorRes: 异或结果数组
//   - m: 变换大小(2的幂)
//   - skewLUT: 倾斜因子查找表
//   - o: 选项配置
func ifftDITEncoder(data [][]byte, mtrunc int, work [][]byte, xorRes [][]byte, m int, skewLUT []ffe, o *options) {
	// 将数据复制到工作空间
	// 注:尝试将memcpy/memset合并到FFT第一层只能提升4%性能,不值得增加复杂度
	for i := 0; i < mtrunc; i++ {
		copy(work[i], data[i]) // 复制有效数据分片
	}
	for i := mtrunc; i < m; i++ {
		memclr(work[i]) // 清零剩余工作空间
	}

	// 注:尝试将前几层分割成L3缓存大小的块只能提升5%性能,不值得增加复杂度

	// 按时间抽取:每次展开2层
	dist := 1  // 基本步长
	dist4 := 4 // 4倍步长
	for dist4 <= m {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend]        // 获取第一组倾斜因子
			log_m02 := skewLUT[iend+dist]   // 获取第二组倾斜因子
			log_m23 := skewLUT[iend+dist*2] // 获取第三组倾斜因子

			// 对每组dist个元素执行4点IFFT
			for i := r; i < iend; i++ {
				ifftDIT4(
					work[i:],
					dist,
					log_m01,
					log_m23,
					log_m02,
					o,
				)
			}
		}

		dist = dist4 // 更新基本步长
		dist4 <<= 2  // 更新4倍步长
		// 注:尝试交替左右扫描以减少缓存未命中,只能提升1%性能,不值得增加复杂度
	}

	// 处理剩余的一层(如果存在)
	if dist < m {
		// 确保dist = m/2
		if dist*2 != m {
			panic("internal error")
		}

		logm := skewLUT[dist] // 获取倾斜因子

		// 根据倾斜因子选择处理方式
		if logm == modulus {
			slicesXor(work[dist:dist*2], work[:dist], o) // 执行异或运算
		} else {
			// 执行2点IFFT
			for i := 0; i < dist; i++ {
				ifftDIT2(work[i], work[i+dist], logm, o)
			}
		}
	}

	// 如果需要,将结果与xorRes异或
	// 注:尝试展开此处循环对16位有限域只能提升5%性能,不值得增加复杂度
	if xorRes != nil {
		slicesXor(xorRes[:m], work[:m], o)
	}
}

// ifftDIT4Ref 执行4点IFFT变换的参考实现
// 参数:
//   - work: 工作空间,包含输入和输出数据
//   - dist: 基本步长
//   - log_m01: 第一层第一组倾斜因子
//   - log_m23: 第一层第二组倾斜因子
//   - log_m02: 第二层倾斜因子
//   - o: 编码器配置选项
func ifftDIT4Ref(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	// 第一层变换:
	if log_m01 == modulus {
		// 如果倾斜因子为模数,执行异或运算
		sliceXor(work[0], work[dist], o)
	} else {
		// 否则执行2点IFFT变换
		ifftDIT2(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus {
		// 如果倾斜因子为模数,执行异或运算
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		// 否则执行2点IFFT变换
		ifftDIT2(work[dist*2], work[dist*3], log_m23, o)
	}

	// 第二层变换:
	if log_m02 == modulus {
		// 如果倾斜因子为模数,执行两次异或运算
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		// 否则执行两次2点IFFT变换
		ifftDIT2(work[0], work[dist*2], log_m02, o)
		ifftDIT2(work[dist], work[dist*3], log_m02, o)
	}
}

// refMulAdd 执行乘加运算的参考实现: x[] ^= y[] * log_m
// 参数:
//   - x: 目标数组,结果将累加到此数组
//   - y: 源数组
//   - log_m: 乘数的对数值
func refMulAdd(x, y []byte, log_m ffe) {
	// 获取对应的乘法查找表
	lut := &mul16LUTs[log_m]

	// 每次处理64字节
	for len(x) >= 64 {
		// 将输入分为高32字节和低32字节
		hiA := y[32:64]
		loA := y[:32]
		dst := x[:64] // 目标缓冲区,64字节

		// 对每个字节对执行乘加运算
		for i, lo := range loA {
			hi := hiA[i]
			// 使用查找表计算乘积
			prod := lut.Lo[lo] ^ lut.Hi[hi]

			// 将16位结果分别异或到目标的低字节和高字节
			dst[i] ^= byte(prod)
			dst[i+32] ^= byte(prod >> 8)
		}
		// 移动指针到下一个64字节块
		x = x[64:]
		y = y[64:]
	}
}

// memclr 清零字节切片
// 参数:
//   - s: 需要清零的字节切片
func memclr(s []byte) {
	// 遍历切片将每个字节设为0
	for i := range s {
		s[i] = 0
	}
}

// slicesXor 对两个二维字节切片执行异或运算
// 参数:
//   - v1: 第一个二维字节切片
//   - v2: 第二个二维字节切片
//   - o: 选项参数
func slicesXor(v1, v2 [][]byte, o *options) {
	// 遍历v1中的每个切片,与v2中对应位置的切片执行异或运算
	for i, v := range v1 {
		sliceXor(v2[i], v, o)
	}
}

// refMul 执行乘法运算的参考实现: x[] = y[] * log_m
// 参数:
//   - x: 目标数组,存储乘法结果
//   - y: 源数组
//   - log_m: 乘数的对数值
func refMul(x, y []byte, log_m ffe) {
	// 获取对应的乘法查找表
	lut := &mul16LUTs[log_m]

	// 每次处理64字节的数据块
	for off := 0; off < len(x); off += 64 {
		// 将输入分为低32字节和高32字节
		loA := y[off : off+32]
		hiA := y[off+32:]
		hiA = hiA[:len(loA)]
		// 对每个字节对执行乘法运算
		for i, lo := range loA {
			hi := hiA[i]
			// 使用查找表计算乘积
			prod := lut.Lo[lo] ^ lut.Hi[hi]

			// 将16位结果分别存储到目标的低字节和高字节
			x[off+i] = byte(prod)
			x[off+i+32] = byte(prod >> 8)
		}
	}
}

// mulLog 计算有限域中的对数乘法: a * Log(b)
// 参数:
//   - a: 第一个操作数
//   - log_b: 第二个操作数的对数值
//
// 返回值:
//   - ffe: 乘法结果
func mulLog(a, log_b ffe) ffe {
	/*
	   注意这不是有限域中的普通乘法运算,因为右操作数已经是对数形式。
	   这样设计是为了将K次表查找操作从性能关键的Decode()方法移到
	   初始化步骤中。LogWalsh[]表包含预计算的对数值,
	   因此用这种形式来进行所有其他乘法运算会更容易。
	*/

	// 如果a为0,则结果为0
	if a == 0 {
		return 0
	}
	// 计算a的对数与log_b的和,然后通过指数表获取最终结果
	return expLUT[addMod(logLUT[a], log_b)]
}

// addMod 在有限域中执行模加法运算: z = x + y (mod kModulus)
// 参数:
//   - a: 第一个操作数
//   - b: 第二个操作数
//
// 返回值:
//   - ffe: 模加法的结果
func addMod(a, b ffe) ffe {
	// 将两个操作数相加
	sum := uint(a) + uint(b)

	// 执行部分约简步骤,允许返回kModulus
	// 通过位移运算实现快速模运算
	return ffe(sum + sum>>bitwidth)
}

// subMod 在有限域中执行模减法运算: z = x - y (mod kModulus)
// 参数:
//   - a: 第一个操作数(被减数)
//   - b: 第二个操作数(减数)
//
// 返回值:
//   - ffe: 模减法的结果
func subMod(a, b ffe) ffe {
	// 执行减法运算
	dif := uint(a) - uint(b)

	// 执行部分约简步骤,允许返回kModulus
	// 通过位移运算实现快速模运算
	return ffe(dif + dif>>bitwidth)
}

// ceilPow2 计算大于或等于n的最小2的幂
// 参数:
//   - n: 输入数值
//
// 返回值:
//   - int: 大于等于n的最小2的幂
func ceilPow2(n int) int {
	// 获取系统字长(以位为单位)
	const w = int(unsafe.Sizeof(n) * 8)
	// 通过位运算计算2的幂
	return 1 << (w - bits.LeadingZeros(uint(n-1)))
}

// fwht 执行快速Walsh-Hadamard变换(时域抽取版本)
// 通过展开成对层来在寄存器中执行跨层操作
// 参数:
//   - data: 待变换的数据数组指针
//   - mtrunc: 数据前端非零元素的数量
func fwht(data *[order]ffe, mtrunc int) {
	// 在时域中抽取:每次展开2层
	dist := 1
	dist4 := 4
	// 当dist4小于等于order时循环处理
	for dist4 <= order {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			// 对每组dist个元素进行处理
			// 使用16位索引以避免[65536]ffe的边界检查
			dist := uint16(dist)
			off := uint16(r)
			for i := uint16(0); i < dist; i++ {
				// 内联fwht4函数的实现
				// 读取值比更新指针更快
				t0 := data[off]
				t1 := data[off+dist]
				t2 := data[off+dist*2]
				t3 := data[off+dist*3]

				// 执行蝶形运算
				t0, t1 = fwht2alt(t0, t1)
				t2, t3 = fwht2alt(t2, t3)
				t0, t2 = fwht2alt(t0, t2)
				t1, t3 = fwht2alt(t1, t3)

				// 存储变换结果
				data[off] = t0
				data[off+dist] = t1
				data[off+dist*2] = t2
				data[off+dist*3] = t3
				off++
			}
		}
		// 更新距离参数
		dist = dist4
		dist4 <<= 2
	}
}

// fwht4 执行4点快速Walsh-Hadamard变换
// 参数:
//   - data: 待变换的数据切片
//   - s: 步长,用于计算数据点之间的间隔
func fwht4(data []ffe, s int) {
	// 计算两倍步长
	s2 := s << 1

	// 获取4个数据点的指针
	t0 := &data[0]    // 第一个数据点
	t1 := &data[s]    // 第二个数据点,间隔s
	t2 := &data[s2]   // 第三个数据点,间隔2s
	t3 := &data[s2+s] // 第四个数据点,间隔3s

	// 执行蝶形运算
	fwht2(t0, t1) // 对第一组两点执行变换
	fwht2(t2, t3) // 对第二组两点执行变换
	fwht2(t0, t2) // 对结果的第一组两点执行变换
	fwht2(t1, t3) // 对结果的第二组两点执行变换
}

// fwht2 执行2点快速Walsh-Hadamard变换,直接修改输入值
// 计算 {a, b} = {a + b, a - b} (模Q)
// 参数:
//   - a: 第一个数据点的指针
//   - b: 第二个数据点的指针
func fwht2(a, b *ffe) {
	sum := addMod(*a, *b) // 计算两点之和
	dif := subMod(*a, *b) // 计算两点之差
	*a = sum              // 更新第一个点为和
	*b = dif              // 更新第二个点为差
}

// fwht2alt 执行2点快速Walsh-Hadamard变换,返回计算结果
// 功能与fwht2相同,但返回结果而不是修改输入
// 参数:
//   - a: 第一个数据点
//   - b: 第二个数据点
//
// 返回值:
//   - ffe: 两点之和
//   - ffe: 两点之差
func fwht2alt(a, b ffe) (ffe, ffe) {
	return addMod(a, b), subMod(a, b)
}

// 用于确保常量只初始化一次的同步对象
var initOnce sync.Once

// initConstants 初始化所有需要的常量表
// 使用sync.Once确保只执行一次初始化
func initConstants() {
	initOnce.Do(func() {
		initLUTs()     // 初始化查找表
		initFFTSkew()  // 初始化FFT偏移量
		initMul16LUT() // 初始化16位乘法查找表
	})
}

// initLUTs 初始化对数表(logLUT)和指数表(expLUT)
// 这些查找表用于在有限域中进行快速乘法运算
// 使用Cantor基底生成查找表,确保运算的正确性和高效性
func initLUTs() {
	// 定义Cantor基底常量数组,用于生成有限域元素
	cantorBasis := [bitwidth]ffe{
		0x0001, 0xACCA, 0x3C0E, 0x163E, // 前4个基底元素
		0xC582, 0xED2E, 0x914C, 0x4012, // 中间4个基底元素
		0x6C98, 0x10D8, 0x6A72, 0xB900, // 后4个基底元素
		0xFDB8, 0xFB34, 0xFF38, 0x991E, // 最后4个基底元素
	}

	// 初始化指数表和对数表的内存空间
	expLUT = &[order]ffe{} // 分配指数表内存
	logLUT = &[order]ffe{} // 分配对数表内存

	// 使用LFSR(线性反馈移位寄存器)生成指数表
	state := 1 // 初始状态设为1
	for i := ffe(0); i < modulus; i++ {
		expLUT[state] = i // 记录当前状态对应的指数
		state <<= 1       // 状态左移一位
		if state >= order {
			state ^= polynomial // 如果超出范围,进行多项式归约
		}
	}
	expLUT[0] = modulus // 设置0元素的指数值

	// 转换为Cantor基底表示
	logLUT[0] = 0 // 初始化对数表的0元素
	for i := 0; i < bitwidth; i++ {
		basis := cantorBasis[i] // 获取当前基底元素
		width := 1 << i         // 计算当前步长

		for j := 0; j < width; j++ {
			logLUT[j+width] = logLUT[j] ^ basis // 使用异或运算生成新的对数表元素
		}
	}

	// 将对数表中的值转换为最终的对数值
	for i := 0; i < order; i++ {
		logLUT[i] = expLUT[logLUT[i]] // 通过指数表转换对数值
	}

	// 构建最终的指数表
	for i := 0; i < order; i++ {
		expLUT[logLUT[i]] = ffe(i) // 将对数和指数的关系对应起来
	}

	// 设置模数位置的值等于0位置的值
	expLUT[modulus] = expLUT[0] // 确保模运算的正确性
}

// initFFTSkew 初始化FFT倾斜因子和Walsh-Hadamard变换所需的查找表
// 该函数用于生成快速傅里叶变换(FFT)中需要的倾斜向量和预计算的Walsh-Hadamard变换值
func initFFTSkew() {
	// 创建临时数组用于存储中间计算结果
	var temp [bitwidth - 1]ffe

	// 生成FFT倾斜向量的初始值
	// 通过位移运算生成2的幂次序列
	for i := 1; i < bitwidth; i++ {
		temp[i-1] = ffe(1 << i)
	}

	// 分配FFT倾斜因子和Walsh对数表的内存空间
	fftSkew = &[modulus]ffe{} // 初始化FFT倾斜因子数组
	logWalsh = &[order]ffe{}  // 初始化Walsh对数变换数组

	// 主循环:计算FFT倾斜因子
	for m := 0; m < bitwidth-1; m++ {
		step := 1 << (m + 1) // 计算步长,为2的m+1次方

		fftSkew[1<<m-1] = 0 // 设置特定位置的倾斜因子为0

		// 计算当前层的倾斜因子值
		for i := m; i < bitwidth-1; i++ {
			s := 1 << (i + 1) // 计算当前迭代的范围

			// 使用异或运算更新倾斜因子数组
			for j := 1<<m - 1; j < s; j += step {
				fftSkew[j+s] = fftSkew[j] ^ temp[i]
			}
		}

		// 更新临时数组中的值
		temp[m] = modulus - logLUT[mulLog(temp[m], logLUT[temp[m]^1])]

		// 更新后续位置的临时值
		for i := m + 1; i < bitwidth-1; i++ {
			sum := addMod(logLUT[temp[i]^1], temp[m]) // 计算模加和
			temp[i] = mulLog(temp[i], sum)            // 更新临时数组元素
		}
	}

	// 将倾斜因子转换为对数域
	for i := 0; i < modulus; i++ {
		fftSkew[i] = logLUT[fftSkew[i]]
	}

	// 预计算Walsh-Hadamard变换的对数值
	// 将对数查找表的值复制到Walsh变换数组
	for i := 0; i < order; i++ {
		logWalsh[i] = logLUT[i]
	}
	logWalsh[0] = 0 // 设置首个元素为0

	// 对logWalsh数组执行快速Walsh-Hadamard变换
	fwht(logWalsh, order)
}

// initMul16LUT 初始化乘法查找表
// 用于优化有限域乘法运算的性能
func initMul16LUT() {
	// 为乘法查找表分配内存空间
	mul16LUTs = &[order]mul16LUT{}

	// 对每个对数乘数进行处理
	for log_m := 0; log_m < order; log_m++ {
		// 临时存储计算结果的数组
		var tmp [64]ffe

		// 处理4个nibble(4位),每个nibble左移不同位数
		for nibble, shift := 0, 0; nibble < 4; {
			// 获取当前nibble对应的查找表切片
			nibble_lut := tmp[nibble*16:]

			// 计算当前nibble的所有可能值(0-15)与log_m的乘积
			for xnibble := 0; xnibble < 16; xnibble++ {
				prod := mulLog(ffe(xnibble<<shift), ffe(log_m))
				nibble_lut[xnibble] = prod
			}
			nibble++
			shift += 4 // 每个nibble移位4位
		}

		// 将临时结果组合到最终的查找表中
		lut := &mul16LUTs[log_m]
		for i := range lut.Lo[:] {
			// 组合低位和中位结果
			lut.Lo[i] = tmp[i&15] ^ tmp[((i>>4)+16)]
			// 组合高位结果
			lut.Hi[i] = tmp[((i&15)+32)] ^ tmp[((i>>4)+48)]
		}
	}

	// 如果CPU支持SSSE3、AVX2或AVX512F指令集,创建256位优化的查找表
	if cpuid.CPU.Has(cpuid.SSSE3) || cpuid.CPU.Has(cpuid.AVX2) || cpuid.CPU.Has(cpuid.AVX512F) {
		// 为256位查找表分配内存
		multiply256LUT = &[order][16 * 8]byte{}

		// 为每个对数乘数创建查找表
		for logM := range multiply256LUT[:] {
			shift := 0
			// 处理4个4位组
			for i := 0; i < 4; i++ {
				// 构造用于PSHUFB指令的16项查找表
				prodLo := multiply256LUT[logM][i*16 : i*16+16]
				prodHi := multiply256LUT[logM][4*16+i*16 : 4*16+i*16+16]
				// 计算每个可能值的乘积并分别存储高8位和低8位
				for x := range prodLo[:] {
					prod := mulLog(ffe(x<<shift), ffe(logM))
					prodLo[x] = byte(prod)
					prodHi[x] = byte(prod >> 8)
				}
				shift += 4
			}
		}
	}
}

// 常量定义
const kWordMips = 5                  // Word级别的MIPS数量
const kWords = order / 64            // 每个Word包含的位数
const kBigMips = 6                   // BigWord级别的MIPS数量
const kBigWords = (kWords + 63) / 64 // BigWord的数量
const kBiggestMips = 4               // BiggestWord级别的MIPS数量
// errorBitfield 包含用于指示需要重建的分片的渐进式错误位图
// 使用多级位图结构来优化查询效率
type errorBitfield struct {
	Words        [kWordMips][kWords]uint64   // 第一级位图,每个word存储64个分片的错误状态
	BigWords     [kBigMips][kBigWords]uint64 // 第二级位图,每个bigword聚合64个word的信息
	BiggestWords [kBiggestMips]uint64        // 第三级位图,每个biggestword聚合所有bigword的信息
}

// set 设置指定位置的错误标记
// 参数:
//   - i: 需要标记错误的分片索引
func (e *errorBitfield) set(i int) {
	// 在第一级位图中设置对应位置的错误标记
	e.Words[0][i/64] |= uint64(1) << (i & 63)
}

// isNeededFn 返回一个函数,用于检查指定MIP级别下某个位是否需要处理
// 参数:
//   - mipLevel: MIP级别(0-16)
//
// 返回:
//   - func(bit int) bool: 返回检查函数
func (e *errorBitfield) isNeededFn(mipLevel int) func(bit int) bool {
	// MIP级别>=16时,所有位都需要处理
	if mipLevel >= 16 {
		return func(bit int) bool {
			return true
		}
	}
	// MIP级别>=12时,使用BiggestWords进行检查
	if mipLevel >= 12 {
		w := e.BiggestWords[mipLevel-12]
		return func(bit int) bool {
			bit /= 4096
			return 0 != (w & (uint64(1) << bit))
		}
	}
	// MIP级别>=6时,使用BigWords进行检查
	if mipLevel >= 6 {
		w := e.BigWords[mipLevel-6][:]
		return func(bit int) bool {
			bit /= 64
			return 0 != (w[bit/64] & (uint64(1) << (bit & 63)))
		}
	}
	// MIP级别>0时,使用Words进行检查
	if mipLevel > 0 {
		w := e.Words[mipLevel-1][:]
		return func(bit int) bool {
			return 0 != (w[bit/64] & (uint64(1) << (bit & 63)))
		}
	}
	return nil
}

// isNeeded 检查指定MIP级别下某个位是否需要处理
// 参数:
//   - mipLevel: MIP级别(0-16)
//   - bit: 要检查的位索引
//
// 返回:
//   - bool: 如果需要处理返回true,否则返回false
func (e *errorBitfield) isNeeded(mipLevel int, bit uint) bool {
	// MIP级别>=16时,所有位都需要处理
	if mipLevel >= 16 {
		return true
	}
	// MIP级别>=12时,使用BiggestWords进行检查
	if mipLevel >= 12 {
		bit /= 4096
		return 0 != (e.BiggestWords[mipLevel-12] & (uint64(1) << bit))
	}
	// MIP级别>=6时,使用BigWords进行检查
	if mipLevel >= 6 {
		bit /= 64
		return 0 != (e.BigWords[mipLevel-6][bit/64] & (uint64(1) << (bit % 64)))
	}
	// 其他情况使用Words进行检查
	return 0 != (e.Words[mipLevel-1][bit/64] & (uint64(1) << (bit % 64)))
}

// kHiMasks 定义了用于位操作的掩码数组
// 每个掩码用于不同级别的位移操作
var kHiMasks = [5]uint64{
	0xAAAAAAAAAAAAAAAA, // 交替的1和0位模式
	0xCCCCCCCCCCCCCCCC, // 每2位交替的1和0模式
	0xF0F0F0F0F0F0F0F0, // 每4位交替的1和0模式
	0xFF00FF00FF00FF00, // 每8位交替的1和0模式
	0xFFFF0000FFFF0000, // 每16位交替的1和0模式
}

// prepare 准备错误位字段的各个MIP级别
// 通过位操作和移位计算生成不同级别的错误位图
func (e *errorBitfield) prepare() {
	// First mip level is for final layer of FFT: pairs of data
	// 处理第一个MIP级别,用于FFT的最后一层:数据对
	for i := 0; i < kWords; i++ {
		// 获取当前word
		w_i := e.Words[0][i]
		// 计算高位到低位的传播
		hi2lo0 := w_i | ((w_i & kHiMasks[0]) >> 1)
		// 计算低位到高位的传播
		lo2hi0 := (w_i & (kHiMasks[0] >> 1)) << 1
		// 合并结果
		w_i = hi2lo0 | lo2hi0
		e.Words[0][i] = w_i

		bits := 2
		// 处理剩余的word MIP级别
		for j := 1; j < kWordMips; j++ {
			// 计算高位到低位的传播
			hi2lo_j := w_i | ((w_i & kHiMasks[j]) >> bits)
			// 计算低位到高位的传播
			lo2hi_j := (w_i & (kHiMasks[j] >> bits)) << bits
			// 合并结果
			w_i = hi2lo_j | lo2hi_j
			e.Words[j][i] = w_i
			bits <<= 1
		}
	}

	// 处理BigWords级别
	for i := 0; i < kBigWords; i++ {
		w_i := uint64(0)
		bit := uint64(1)
		// 获取源数据
		src := e.Words[kWordMips-1][i*64 : i*64+64]
		// 压缩64个word到一个uint64中
		for _, w := range src {
			w_i |= (w | (w >> 32) | (w << 32)) & bit
			bit <<= 1
		}
		e.BigWords[0][i] = w_i

		bits := 1
		// 处理剩余的BigWords MIP级别
		for j := 1; j < kBigMips; j++ {
			// 计算高位到低位的传播
			hi2lo_j := w_i | ((w_i & kHiMasks[j-1]) >> bits)
			// 计算低位到高位的传播
			lo2hi_j := (w_i & (kHiMasks[j-1] >> bits)) << bits
			// 合并结果
			w_i = hi2lo_j | lo2hi_j
			e.BigWords[j][i] = w_i
			bits <<= 1
		}
	}

	// 处理BiggestWords级别
	w_i := uint64(0)
	bit := uint64(1)
	// 压缩所有BigWords到一个uint64中
	for _, w := range e.BigWords[kBigMips-1][:kBigWords] {
		w_i |= (w | (w >> 32) | (w << 32)) & bit
		bit <<= 1
	}
	e.BiggestWords[0] = w_i

	bits := uint64(1)
	// 处理剩余的BiggestWords MIP级别
	for j := 1; j < kBiggestMips; j++ {
		// 计算高位到低位的传播
		hi2lo_j := w_i | ((w_i & kHiMasks[j-1]) >> bits)
		// 计算低位到高位的传播
		lo2hi_j := (w_i & (kHiMasks[j-1] >> bits)) << bits
		// 合并结果
		w_i = hi2lo_j | lo2hi_j
		e.BiggestWords[j] = w_i
		bits <<= 1
	}
}

// fftDIT 执行时域抽取的快速傅里叶变换
// 参数:
//   - work: 工作数组,存储FFT计算的中间结果
//   - mtrunc: 截断长度
//   - m: FFT点数
//   - skewLUT: 倾斜因子查找表
//   - o: 配置选项
func (e *errorBitfield) fftDIT(work [][]byte, mtrunc, m int, skewLUT []ffe, o *options) {
	// 计算MIP级别,为FFT点数的以2为底的对数减1
	mipLevel := bits.Len32(uint32(m)) - 1

	// 初始化距离参数
	dist4 := m     // 4个元素一组的距离
	dist := m >> 2 // 单个元素的距离
	// 获取当前MIP级别需要处理的元素判断函数
	needed := e.isNeededFn(mipLevel)

	// 主循环:每次处理2层FFT
	for dist != 0 {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			// 跳过不需要处理的元素
			if !needed(r) {
				continue
			}
			// 计算当前组的结束位置
			iEnd := r + dist
			// 获取倾斜因子
			logM01 := skewLUT[iEnd-1]        // 第0-1层的倾斜因子
			logM02 := skewLUT[iEnd+dist-1]   // 第0-2层的倾斜因子
			logM23 := skewLUT[iEnd+dist*2-1] // 第2-3层的倾斜因子

			// 对每组dist个元素执行4点FFT
			for i := r; i < iEnd; i++ {
				fftDIT4(
					work[i:], // 输入数据
					dist,     // 距离
					logM01,   // 0-1层倾斜因子
					logM23,   // 2-3层倾斜因子
					logM02,   // 0-2层倾斜因子
					o)        // 配置选项
			}
		}
		// 更新距离和MIP级别参数
		dist4 = dist                    // 更新4点距离
		dist >>= 2                      // 距离除以4
		mipLevel -= 2                   // MIP级别减2
		needed = e.isNeededFn(mipLevel) // 更新需要处理的元素判断函数
	}

	// 处理剩余的一层(如果存在)
	if dist4 == 2 {
		// 对每组2个元素进行处理
		for r := 0; r < mtrunc; r += 2 {
			// 跳过不需要处理的元素
			if !needed(r) {
				continue
			}
			// 获取倾斜因子
			logM := skewLUT[r+1-1]

			// 根据倾斜因子选择处理方式
			if logM == modulus {
				// 如果倾斜因子等于模数,执行异或操作
				sliceXor(work[r], work[r+1], o)
			} else {
				// 否则执行2点FFT
				fftDIT2(work[r], work[r+1], logM, o)
			}
		}
	}
}
