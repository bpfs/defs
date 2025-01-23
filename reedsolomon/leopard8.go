package reedsolomon

// 这是一个O(n*log n)复杂度的Reed-Solomon码实现,
// 移植自C++库 https://github.com/catid/leopard.
//
// 该实现基于以下论文:
//
// S.-J. Lin, T. Y. Al-Naffouri, Y. S. Han, 和 W.-H. Chung,
// "基于快速傅里叶变换的新型多项式基及其在Reed-Solomon纠删码中的应用"
// IEEE 信息理论汇刊, pp. 6284-6299, 2016年11月.

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/bits"
	"sync"
)

// leopardFF8 实现了一个基于8位有限域的Reed-Solomon编码器
// 该实现基于"leopard"算法,用于处理最多256个分片的情况
type leopardFF8 struct {
	// dataShards 表示数据分片的数量
	// 这个值在初始化后不应被修改
	dataShards int

	// parityShards 表示校验分片的数量
	// 这个值在初始化后不应被修改
	parityShards int

	// totalShards 表示所有分片的总数
	// 由 dataShards + parityShards 计算得出
	// 这个值在初始化后不应被修改
	totalShards int

	// workPool 是一个对象池,用于重用临时工作缓冲区
	// 这可以减少内存分配和GC压力
	workPool sync.Pool

	// inversion 用于缓存GF8域的反演计算结果
	// key为反演输入,value为计算结果
	inversion map[[inversion8Bytes]byte]leopardGF8cache

	// inversionMu 用于保护inversion map的并发访问
	inversionMu sync.Mutex

	// o 包含编码器的配置选项
	o options
}

// inversion8Bytes 定义了反演缓存键的大小
// 256位(32字节)用于存储错误位置信息
const inversion8Bytes = 256 / 8

// leopardGF8cache 定义了GF8域反演计算的缓存结构
type leopardGF8cache struct {
	// errorLocs 存储错误位置信息
	errorLocs [256]ffe8
	// bits 存储错误位域信息
	bits *errorBitfield8
}

// newFF8 创建一个新的8位leopard编码器实例
// 参数:
//   - dataShards: 数据分片数量
//   - parityShards: 校验分片数量
//   - opt: 编码器配置选项
//
// 返回值:
//   - *leopardFF8: 创建的编码器实例
//   - error: 错误信息,如果参数无效则返回错误
func newFF8(dataShards, parityShards int, opt options) (*leopardFF8, error) {
	// 初始化编码所需的常量表
	initConstants8()

	// 验证分片数量是否有效
	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvShardNum
	}

	// 验证总分片数是否超过最大限制65536
	if dataShards+parityShards > 65536 {
		return nil, ErrMaxShardNum
	}

	// 创建并初始化leopardFF8实例
	r := &leopardFF8{
		dataShards:   dataShards,                // 设置数据分片数
		parityShards: parityShards,              // 设置校验分片数
		totalShards:  dataShards + parityShards, // 计算总分片数
		o:            opt,                       // 设置编码器选项
	}

	// 如果启用了反演缓存且总分片数较小或强制使用缓存
	if opt.inversionCache && (r.totalShards <= 64 || opt.forcedInversionCache) {
		// 为大分片数量时缓存效果较差且占用内存较多
		// totalShards只是空间估计值而非实际覆盖范围
		r.inversion = make(map[[inversion8Bytes]byte]leopardGF8cache, r.totalShards)
	}

	return r, nil // 返回创建的实例
}

// 确保leopardFF8实现了Extensions接口
var _ = Extensions(&leopardFF8{})

// ShardSizeMultiple 返回分片大小必须是其倍数的值
// 返回值:
//   - int: 分片大小倍数,固定为64
func (r *leopardFF8) ShardSizeMultiple() int {
	return 64 // 返回固定值64作为分片大小的倍数
}

// DataShards 返回数据分片的数量
// 返回值:
//   - int: 数据分片数量
func (r *leopardFF8) DataShards() int {
	return r.dataShards // 返回编码器中的数据分片数量
}

// ParityShards 返回校验分片的数量
// 返回值:
//   - int: 校验分片数量
func (r *leopardFF8) ParityShards() int {
	return r.parityShards // 返回编码器中的校验分片数量
}

// TotalShards 返回总分片数量
// 返回值:
//   - int: 总分片数量(数据分片+校验分片)
func (r *leopardFF8) TotalShards() int {
	return r.totalShards // 返回编码器中的总分片数量
}

// AllocAligned 分配对齐的内存空间用于存储分片数据
// 参数:
//   - each: 每个分片的大小
//
// 返回值:
//   - [][]byte: 分配的二维字节切片
func (r *leopardFF8) AllocAligned(each int) [][]byte {
	return AllocAligned(r.totalShards, each) // 调用全局AllocAligned函数分配内存
}

// ffe8 是一个8位无符号整数类型,用于有限域运算
type ffe8 uint8

// 定义有限域运算相关的常量
const (
	bitwidth8   = 8              // 位宽度
	order8      = 1 << bitwidth8 // 有限域的阶(大小)
	modulus8    = order8 - 1     // 模数
	polynomial8 = 0x11D          // 生成多项式

	workSize8 = 32 << 10 // 编码块大小,32KB
)

// 定义FFT运算所需的查找表
var (
	fftSkew8  *[modulus8]ffe8 // FFT倾斜因子表
	logWalsh8 *[order8]ffe8   // Walsh变换的对数表
)

// 定义对数运算所需的查找表
var (
	logLUT8 *[order8]ffe8 // 对数查找表
	expLUT8 *[order8]ffe8 // 指数查找表
)

// mul8LUTs 存储x * y的部分积,偏移量为x + y * 256
// 对相同y值的重复访问更快
var mul8LUTs *[order8]mul8LUT

// mul8LUT 定义8位乘法查找表的结构
type mul8LUT struct {
	Value [256]ffe8 // 乘积查找表
}

// multiply256LUT8 存储AVX2指令集优化的查找表
var multiply256LUT8 *[order8][2 * 16]byte

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
func (r *leopardFF8) Encode(shards [][]byte) error {
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
func (r *leopardFF8) encode(shards [][]byte) error {
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
	} else {
		// 如果对象池为空,分配新的工作空间
		work = AllocAligned(m*2, workSize8)
	}

	// 确保工作空间足够大
	if cap(work) >= m*2 {
		work = work[:m*2]
		// 初始化工作空间中的每个分片
		for i := range work {
			if i >= r.parityShards {
				if cap(work[i]) < workSize8 {
					work[i] = AllocAligned(1, workSize8)[0]
				} else {
					work[i] = work[i][:workSize8]
				}
			}
		}
	} else {
		// 分配新的工作空间
		work = AllocAligned(m*2, workSize8)
	}

	// 编码完成后将工作空间放回对象池
	defer r.workPool.Put(&work)

	// 计算实际需要处理的分片数
	mtrunc := m
	if r.dataShards < mtrunc {
		mtrunc = r.dataShards
	}

	// 获取FFT倾斜因子表
	skewLUT := fftSkew8[m-1:]

	// 初始化偏移量和临时分片数组
	off := 0
	sh := make([][]byte, len(shards))

	// 创建可修改的工作空间副本
	wMod := make([][]byte, len(work))
	copy(wMod, work)

	// 按块处理大型分片
	for off < shardSize {
		work := wMod
		sh := sh
		// 计算当前块的结束位置
		end := off + workSize8
		if end > shardSize {
			end = shardSize
			sz := shardSize - off
			// 调整最后一块的大小
			for i := range work {
				work[i] = work[i][:sz]
			}
		}
		// 设置每个分片的当前处理范围
		for i := range shards {
			sh[i] = shards[i][off:end]
		}

		// 将输出直接写入校验分片
		res := shards[r.dataShards:r.totalShards]
		for i := range res {
			work[i] = res[i][off:end]
		}

		// 对数据分片执行IFFT变换
		ifftDITEncoder8(
			sh[:r.dataShards],
			mtrunc,
			work,
			nil, // 不需要异或输出
			m,
			skewLUT,
			&r.o,
		)

		// 计算最后一组不完整分片的数量
		lastCount := r.dataShards % m
		skewLUT2 := skewLUT
		if m >= r.dataShards {
			goto skip_body
		}

		// 处理完整的m个数据分片组
		for i := m; i+m <= r.dataShards; i += m {
			sh = sh[m:]
			skewLUT2 = skewLUT2[m:]

			// 对数据执行IFFT并与work异或
			ifftDITEncoder8(
				sh, // 数据源
				m,
				work[m:], // 临时工作空间
				work,     // 异或目标
				m,
				skewLUT2,
				&r.o,
			)
		}

		// 处理最后一组不完整的数据分片
		if lastCount != 0 {
			sh = sh[m:]
			skewLUT2 = skewLUT2[m:]

			// 对剩余数据执行IFFT并与work异或
			ifftDITEncoder8(
				sh, // 数据源
				lastCount,
				work[m:], // 临时工作空间
				work,     // 异或目标
				m,
				skewLUT2,
				&r.o,
			)
		}

	skip_body:
		// 对work执行FFT变换生成校验分片
		fftDIT8(work, r.parityShards, m, fftSkew8[:], &r.o)
		// 更新偏移量,处理下一块
		off += workSize8
	}

	return nil
}

// EncodeIdx 对单个数据分片进行编码
// 参数:
//   - dataShard: 输入的数据分片
//   - idx: 分片索引
//   - parity: 校验分片数组
//
// 返回值:
//   - error: 编码错误,当前不支持此操作
func (r *leopardFF8) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	return ErrNotSupported // 返回不支持错误
}

// Join 将多个分片合并成原始数据
// 参数:
//   - dst: 输出目标Writer
//   - shards: 输入分片数组
//   - outSize: 期望输出的数据大小
//
// 返回值:
//   - error: 合并过程中的错误
func (r *leopardFF8) Join(dst io.Writer, shards [][]byte, outSize int) error {
	// 检查是否有足够的分片数量
	if len(shards) < r.dataShards {
		return ErrTooFewShards
	}
	// 只使用数据分片部分
	shards = shards[:r.dataShards]

	// 计算可用数据总大小
	size := 0
	for _, shard := range shards {
		// 检查分片是否为空
		if shard == nil {
			return ErrReconstructRequired
		}
		// 累加分片大小
		size += len(shard)

		// 如果已经达到需要的大小则跳出
		if size >= outSize {
			break
		}
	}
	// 检查数据是否足够
	if size < outSize {
		return ErrShortData
	}

	// 将数据写入目标
	write := outSize
	for _, shard := range shards {
		// 处理最后一个不完整分片
		if write < len(shard) {
			_, err := dst.Write(shard[:write])
			return err
		}
		// 写入完整分片
		n, err := dst.Write(shard)
		if err != nil {
			return err
		}
		// 更新剩余需要写入的数据量
		write -= n
	}
	return nil
}

// Update 更新数据分片并重新生成校验分片
// 参数:
//   - shards: 所有分片数组
//   - newDatashards: 新的数据分片
//
// 返回值:
//   - error: 更新错误,当前不支持此操作
func (r *leopardFF8) Update(shards [][]byte, newDatashards [][]byte) error {
	return ErrNotSupported // 返回不支持错误
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
func (r *leopardFF8) Split(data []byte) ([][]byte, error) {
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
		// 如果容量足够,扩展到需要的总大小
		if cap(data) > needTotal {
			data = data[:needTotal]
		} else {
			// 否则扩展到可用的最大容量
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
		// 如果有不完整的分片需要处理
		if dataLen > perShard*fullShards {
			// 复制剩余的部分数据到新分配的分片中
			copyFrom := data[perShard*fullShards : dataLen]
			for i := range padding {
				if len(copyFrom) == 0 {
					break
				}
				copyFrom = copyFrom[copy(padding[i], copyFrom):]
			}
		}
	}

	// 将数据分割成等长的分片
	dst := make([][]byte, r.totalShards)
	i := 0
	// 先处理完整的分片
	for ; i < len(dst) && len(data) >= perShard; i++ {
		dst[i] = data[:perShard:perShard]
		data = data[perShard:]
	}

	// 使用padding填充剩余的分片
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
func (r *leopardFF8) ReconstructSome(shards [][]byte, required []bool) error {
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
func (r *leopardFF8) Reconstruct(shards [][]byte) error {
	// 调用内部重建方法,重建所有分片
	return r.reconstruct(shards, true)
}

// ReconstructData 仅重建丢失的数据分片
// 参数:
//   - shards: 分片数组,包含数据分片和校验分片
//
// 返回值:
//   - error: 重建失败时返回相应错误,成功返回nil
func (r *leopardFF8) ReconstructData(shards [][]byte) error {
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
func (r *leopardFF8) Verify(shards [][]byte) (bool, error) {
	// 检查分片数量是否正确
	if len(shards) != r.totalShards {
		return false, ErrTooFewShards
	}
	// 检查分片参数是否合法,不允许空分片
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
func (r *leopardFF8) reconstruct(shards [][]byte, recoverAll bool) error {
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

	// 检查是否有足够的分片用于重建
	if numberPresent < r.dataShards {
		return ErrTooFewShards
	}

	// 获取分片大小并检查是否为64的倍数
	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	// 当丢失的校验分片少于1/4且数据量较大时使用位域优化
	useBits := r.totalShards-numberPresent <= r.parityShards/4 && shardSize*r.totalShards >= 64<<10

	// 计算大于等于校验分片数的最小2的幂
	m := ceilPow2(r.parityShards)
	// 计算大于等于m+数据分片数的最小2的幂
	n := ceilPow2(m + r.dataShards)

	// 是否启用错误位域优化
	const LEO_ERROR_BITFIELD_OPT = true

	// 初始化错误位置数组和位域
	var errorBits errorBitfield8
	var errLocs [order8]ffe8
	// 标记丢失的校验分片位置
	for i := 0; i < r.parityShards; i++ {
		if len(shards[i+r.dataShards]) == 0 {
			errLocs[i] = 1
			if LEO_ERROR_BITFIELD_OPT && recoverAll {
				errorBits.set(i)
			}
		}
	}
	// 标记填充的校验分片位置
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

	// 尝试从缓存中获取反演结果
	var gotInversion bool
	if LEO_ERROR_BITFIELD_OPT && r.inversion != nil {
		cacheID := errorBits.cacheID()
		r.inversionMu.Lock()
		if inv, ok := r.inversion[cacheID]; ok {
			r.inversionMu.Unlock()
			errLocs = inv.errorLocs
			if inv.bits != nil && useBits {
				errorBits = *inv.bits
				useBits = true
			} else {
				useBits = false
			}
			gotInversion = true
		} else {
			r.inversionMu.Unlock()
		}
	}

	// 如果没有找到缓存的反演结果,计算新的反演
	if !gotInversion {
		// 如果使用位域优化,准备位域
		if LEO_ERROR_BITFIELD_OPT && useBits {
			errorBits.prepare()
		}

		// 计算错误定位多项式
		fwht8(&errLocs, m+r.dataShards)

		// 应用Walsh变换
		for i := 0; i < order8; i++ {
			errLocs[i] = ffe8((uint(errLocs[i]) * uint(logWalsh8[i])) % modulus8)
		}

		// 再次应用Walsh变换
		fwht8(&errLocs, order8)

		// 缓存计算结果
		if r.inversion != nil {
			c := leopardGF8cache{
				errorLocs: errLocs,
			}
			if useBits {
				var x errorBitfield8
				x = errorBits
				c.bits = &x
			}
			r.inversionMu.Lock()
			r.inversion[errorBits.cacheID()] = c
			r.inversionMu.Unlock()
		}
	}

	// 从对象池获取或创建工作空间
	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
	// 如果工作空间容量足够,重用它
	if cap(work) >= n {
		work = work[:n]
		for i := range work {
			if cap(work[i]) < workSize8 {
				work[i] = make([]byte, workSize8)
			} else {
				work[i] = work[i][:workSize8]
			}
		}
	} else {
		// 创建新的工作空间
		work = make([][]byte, n)
		all := make([]byte, n*workSize8)
		for i := range work {
			work[i] = all[i*workSize8 : i*workSize8+workSize8]
		}
	}
	// 将工作空间放回对象池
	defer r.workPool.Put(work)

	// 创建分片副本用于处理
	sh := make([][]byte, len(shards))
	copy(sh, shards)

	// 为需要恢复的分片分配空间
	for i, sh := range shards {
		if !recoverAll && i >= r.dataShards {
			continue
		}
		if len(sh) == 0 {
			if cap(sh) >= shardSize {
				shards[i] = sh[:shardSize]
			} else {
				shards[i] = make([]byte, shardSize)
			}
		}
	}

	// 分块处理大分片
	off := 0
	for off < shardSize {
		// 计算当前块的结束位置
		endSlice := off + workSize8
		if endSlice > shardSize {
			endSlice = shardSize
			sz := shardSize - off
			// 调整最后一块的大小
			for i := range work {
				work[i] = work[i][:sz]
			}
		}
		// 更新分片切片范围
		for i := range shards {
			if len(sh[i]) != 0 {
				sh[i] = shards[i][off:endSlice]
			}
		}
		// 处理校验分片
		for i := 0; i < r.parityShards; i++ {
			if len(sh[i+r.dataShards]) != 0 {
				mulgf8(work[i], sh[i+r.dataShards], errLocs[i], &r.o)
			} else {
				memclr(work[i])
			}
		}
		// 清空填充的校验分片
		for i := r.parityShards; i < m; i++ {
			memclr(work[i])
		}

		// 处理数据分片
		for i := 0; i < r.dataShards; i++ {
			if len(sh[i]) != 0 {
				mulgf8(work[m+i], sh[i], errLocs[m+i], &r.o)
			} else {
				memclr(work[m+i])
			}
		}
		// 清空剩余工作空间
		for i := m + r.dataShards; i < n; i++ {
			memclr(work[i])
		}

		// 执行IFFT变换
		ifftDITDecoder8(
			m+r.dataShards,
			work,
			n,
			fftSkew8[:],
			&r.o,
		)

		// 计算形式导数
		for i := 1; i < n; i++ {
			width := ((i ^ (i - 1)) + 1) >> 1
			slicesXor(work[i-width:i], work[i:i+width], &r.o)
		}

		// 执行FFT变换
		outputCount := m + r.dataShards

		// 根据是否使用位域优化选择不同的FFT实现
		if LEO_ERROR_BITFIELD_OPT && useBits {
			errorBits.fftDIT8(work, outputCount, n, fftSkew8[:], &r.o)
		} else {
			fftDIT8(work, outputCount, n, fftSkew8[:], &r.o)
		}

		// 恢复丢失的分片
		end := r.dataShards
		if recoverAll {
			end = r.totalShards
		}
		// 恢复每个丢失的分片
		for i := 0; i < end; i++ {
			if len(sh[i]) != 0 {
				continue
			}

			if i >= r.dataShards {
				// 恢复校验分片
				mulgf8(shards[i][off:endSlice], work[i-r.dataShards], modulus8-errLocs[i-r.dataShards], &r.o)
			} else {
				// 恢复数据分片
				mulgf8(shards[i][off:endSlice], work[i+m], modulus8-errLocs[i+m], &r.o)
			}
		}
		// 移动到下一个块
		off += workSize8
	}
	return nil
}

// ifftDITDecoder8 执行解码器的快速傅里叶逆变换(IFFT)
// 参数:
//   - mtrunc: 截断长度,用于限制处理范围
//   - work: 工作空间,存储中间计算结果
//   - m: FFT点数,必须是2的幂
//   - skewLUT: 倾斜因子查找表
//   - o: 编码器选项
func ifftDITDecoder8(mtrunc int, work [][]byte, m int, skewLUT []ffe8, o *options) {
	// 初始化步长参数
	dist := 1  // 基本步长
	dist4 := 4 // 4倍步长,用于两层展开

	// 按时间抽取(DIT)方式执行IFFT,每次展开两层
	for dist4 <= m {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			// 计算当前组的结束位置
			iend := r + dist
			// 从倾斜因子表获取三个变换系数
			log_m01 := skewLUT[iend-1]        // 第一组变换系数
			log_m02 := skewLUT[iend+dist-1]   // 第二组变换系数
			log_m23 := skewLUT[iend+dist*2-1] // 第三组变换系数

			// 对每组dist个元素执行4点IFFT变换
			for i := r; i < iend; i++ {
				ifftDIT48(work[i:], dist, log_m01, log_m23, log_m02, o)
			}
		}
		// 更新步长
		dist = dist4 // 基本步长更新为4倍步长
		dist4 <<= 2  // 4倍步长左移2位(乘4)
	}

	// 处理最后剩余的一层(如果存在)
	if dist < m {
		// 确保dist为m的一半
		if dist*2 != m {
			panic("internal error") // 如果不满足条件则报错
		}

		// 获取最后一层的变换系数
		log_m := skewLUT[dist-1]

		// 根据变换系数选择处理方式
		if log_m == modulus8 {
			// 如果系数等于模数,直接异或运算
			slicesXor(work[dist:2*dist], work[:dist], o)
		} else {
			// 否则对每个元素执行2点IFFT变换
			for i := 0; i < dist; i++ {
				ifftDIT28(
					work[i],      // 第一个输入输出缓冲区
					work[i+dist], // 第二个输入输出缓冲区
					log_m,        // 变换系数
					o,            // 编码器选项
				)
			}
		}
	}
}

// fftDIT8 执行就地FFT变换,用于编码器和解码器
// 参数:
//   - work: 工作缓冲区,存储输入数据和输出结果
//   - mtrunc: 实际需要处理的数据长度
//   - m: FFT变换的大小(2的幂)
//   - skewLUT: 倾斜因子查找表
//   - o: 编码器选项
//
// 说明:
//   - 使用按时间抽取(DIT)方式执行FFT
//   - 每次展开两层进行处理以提高性能
func fftDIT8(work [][]byte, mtrunc, m int, skewLUT []ffe8, o *options) {
	// 按时间抽取:每次展开两层
	dist4 := m     // 4倍步长初始化为m
	dist := m >> 2 // 基本步长初始化为m/4

	// 当基本步长不为0时继续处理
	for dist != 0 {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			// 计算当前组的结束位置
			iend := r + dist
			// 获取三个变换系数
			log_m01 := skewLUT[iend-1]        // 第一组变换系数
			log_m02 := skewLUT[iend+dist-1]   // 第二组变换系数
			log_m23 := skewLUT[iend+dist*2-1] // 第三组变换系数

			// 对每组dist个元素执行4点FFT变换
			for i := r; i < iend; i++ {
				fftDIT48(work[i:], dist, log_m01, log_m23, log_m02, o)
			}
		}
		// 更新步长
		dist4 = dist // 4倍步长更新为当前基本步长
		dist >>= 2   // 基本步长右移2位(除4)
	}

	// 处理最后剩余的一层(如果存在)
	if dist4 == 2 {
		// 对每对元素执行2点FFT变换
		for r := 0; r < mtrunc; r += 2 {
			// 获取变换系数
			log_m := skewLUT[r+1-1]

			// 根据变换系数选择处理方式
			if log_m == modulus8 {
				// 如果系数等于模数,直接异或运算
				sliceXor(work[r], work[r+1], o)
			} else {
				// 否则执行2点FFT变换
				fftDIT28(work[r], work[r+1], log_m, o)
			}
		}
	}
}

// fftDIT4Ref8 执行4点蝶形运算
// 参数:
//   - work: 工作缓冲区,存储输入数据和输出结果
//   - dist: 蝶形运算的步长
//   - log_m01: 第一层第一组变换系数
//   - log_m23: 第一层第二组变换系数
//   - log_m02: 第二层变换系数
//   - o: 编码器选项
//
// 说明:
//   - 实现了4点FFT的基本蝶形运算
//   - 分两层执行变换
func fftDIT4Ref8(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe8, o *options) {
	// 第一层变换:
	if log_m02 == modulus8 {
		// 如果系数等于模数,直接异或运算
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		// 否则执行2点FFT变换
		fftDIT28(work[0], work[dist*2], log_m02, o)
		fftDIT28(work[dist], work[dist*3], log_m02, o)
	}

	// 第二层变换:
	if log_m01 == modulus8 {
		// 如果系数等于模数,直接异或运算
		sliceXor(work[0], work[dist], o)
	} else {
		// 否则执行2点FFT变换
		fftDIT28(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus8 {
		// 如果系数等于模数,直接异或运算
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		// 否则执行2点FFT变换
		fftDIT28(work[dist*2], work[dist*3], log_m23, o)
	}
}

// ifftDITEncoder8 执行编码器的展开IFFT变换
// 参数:
//   - data: 输入数据切片
//   - mtrunc: 有效数据长度
//   - work: 工作缓冲区
//   - xorRes: 异或结果缓冲区,可为nil
//   - m: 总数据长度
//   - skewLUT: 倾斜因子查找表
//   - o: 编码器选项
//
// 说明:
//   - 实现了编码器的快速傅里叶逆变换(IFFT)
//   - 每次展开2层进行计算以提高性能
func ifftDITEncoder8(data [][]byte, mtrunc int, work [][]byte, xorRes [][]byte, m int, skewLUT []ffe8, o *options) {
	// 将输入数据复制到工作缓冲区
	// 注:尝试将memcpy/memset合并到FFT第一层只能提升4%性能,不值得增加复杂度
	for i := 0; i < mtrunc; i++ {
		copy(work[i], data[i])
	}
	// 将剩余空间清零
	for i := mtrunc; i < m; i++ {
		memclr(work[i])
	}

	// 按时间抽取:每次展开2层
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend]        // 第一层第一组变换系数
			log_m02 := skewLUT[iend+dist]   // 第二层变换系数
			log_m23 := skewLUT[iend+dist*2] // 第一层第二组变换系数

			// 对每组dist个元素进行处理
			for i := r; i < iend; i++ {
				ifftDIT48(
					work[i:],
					dist,
					log_m01,
					log_m23,
					log_m02,
					o,
				)
			}
		}

		dist = dist4
		dist4 <<= 2
		// 注:尝试交替左右扫描以减少缓存未命中,
		// 但只能提升1%性能,不值得增加复杂度
	}

	// 如果还剩一层未处理
	if dist < m {
		// 确保dist = m/2
		if dist*2 != m {
			panic("internal error")
		}

		logm := skewLUT[dist]

		if logm == modulus8 {
			// 如果系数等于模数,直接异或运算
			slicesXor(work[dist:dist*2], work[:dist], o)
		} else {
			// 否则执行2点IFFT变换
			for i := 0; i < dist; i++ {
				ifftDIT28(work[i], work[i+dist], logm, o)
			}
		}
	}

	// 如果需要,将结果异或到xorRes中
	// 注:尝试展开循环但对16位有限域只能提升5%性能,不值得增加复杂度
	if xorRes != nil {
		slicesXor(xorRes[:m], work[:m], o)
	}
}

// ifftDIT4Ref8 执行4点IFFT变换的参考实现
// 参数:
//   - work: 待处理的字节切片数组
//   - dist: 相邻元素间的距离
//   - log_m01: 第一层第一组变换系数
//   - log_m23: 第一层第二组变换系数
//   - log_m02: 第二层变换系数
//   - o: 选项参数
func ifftDIT4Ref8(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe8, o *options) {
	// 第一层变换:
	// 处理第一组元素
	if log_m01 == modulus8 {
		// 如果系数等于模数,直接异或运算
		sliceXor(work[0], work[dist], o)
	} else {
		// 否则执行2点IFFT变换
		ifftDIT28(work[0], work[dist], log_m01, o)
	}

	// 处理第二组元素
	if log_m23 == modulus8 {
		// 如果系数等于模数,直接异或运算
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		// 否则执行2点IFFT变换
		ifftDIT28(work[dist*2], work[dist*3], log_m23, o)
	}

	// 第二层变换:
	if log_m02 == modulus8 {
		// 如果系数等于模数,直接异或运算
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		// 否则执行2点IFFT变换
		ifftDIT28(work[0], work[dist*2], log_m02, o)
		ifftDIT28(work[dist], work[dist*3], log_m02, o)
	}
}

// refMulAdd8 执行乘加运算的参考实现: x[] ^= y[] * log_m
// 参数:
//   - x: 目标字节切片,结果将累加到此切片
//   - y: 源字节切片
//   - log_m: 乘法系数的对数值
func refMulAdd8(x, y []byte, log_m ffe8) {
	// 获取乘法查找表
	lut := &mul8LUTs[log_m]

	// 每次处理64字节
	for len(x) >= 64 {
		// 获取源数据和目标数据的64字节片段
		src := y[:64]
		dst := x[:len(src)]
		// 对每个字节执行乘加运算
		for i, y1 := range src {
			dst[i] ^= byte(lut.Value[y1])
		}
		// 移动指针到下一个64字节块
		x = x[64:]
		y = y[64:]
	}
}

// refMul8 执行乘法运算的参考实现: x[] = y[] * log_m
// 参数:
//   - x: 目标字节切片,存储计算结果
//   - y: 源字节切片
//   - log_m: 乘法系数的对数值
func refMul8(x, y []byte, log_m ffe8) {
	// 获取乘法查找表
	lut := &mul8LUTs[log_m]

	// 每次处理64字节
	for off := 0; off < len(x); off += 64 {
		// 获取源数据的64字节片段
		src := y[off : off+64]
		// 对每个字节执行乘法运算
		for i, y1 := range src {
			x[off+i] = byte(lut.Value[y1])
		}
	}
}

// mulLog8 计算有限域中的对数乘法: a * Log(b)
// 参数:
//   - a: 第一个操作数
//   - log_b: 第二个操作数的对数值
//
// 返回值:
//   - ffe8: 乘法结果
//
// 说明:
//   - 这不是普通的有限域乘法,因为右操作数已经是对数形式
//   - 这样做是为了将K次表查找从Decode()方法移到初始化步骤中
//   - LogWalsh[]表包含预计算的对数,所以其他乘法也使用这种形式更容易
func mulLog8(a, log_b ffe8) ffe8 {
	// 如果a为0,直接返回0
	if a == 0 {
		return 0
	}
	// 计算a的对数,与log_b相加后取指数
	return expLUT8[addMod8(logLUT8[a], log_b)]
}

// addMod8 计算模加法: z = x + y (mod kModulus)
// 参数:
//   - a: 第一个操作数
//   - b: 第二个操作数
//
// 返回值:
//   - ffe8: 模加法结果
func addMod8(a, b ffe8) ffe8 {
	// 计算无符号整数加法
	sum := uint(a) + uint(b)
	// 部分约简步骤,允许返回kModulus
	return ffe8(sum + sum>>bitwidth8)
}

// subMod8 计算模减法: z = x - y (mod kModulus)
// 参数:
//   - a: 第一个操作数
//   - b: 第二个操作数
//
// 返回值:
//   - ffe8: 模减法结果
func subMod8(a, b ffe8) ffe8 {
	// 计算无符号整数减法
	dif := uint(a) - uint(b)
	// 部分约简步骤,允许返回kModulus
	return ffe8(dif + dif>>bitwidth8)
}

// fwht8 执行时域快速Walsh-Hadamard变换(DIT-FWHT)
// 将成对的层展开以在寄存器中执行跨层操作
// 参数:
//   - data: 要进行变换的数据数组指针
//   - mtrunc: 数据前端非零元素的数量
func fwht8(data *[order8]ffe8, mtrunc int) {
	// dist表示当前层的距离,dist4表示4倍距离
	dist := 1
	dist4 := 4

	// 每次迭代处理两层,直到dist4超过order8
	for dist4 <= order8 {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			// 使用16位索引避免[65536]ffe8的边界检查
			dist := uint16(dist)
			off := uint16(r)

			// 对每组dist个元素进行处理
			for i := uint16(0); i < dist; i++ {
				// 读取4个元素值
				t0 := data[off]
				t1 := data[off+dist]
				t2 := data[off+dist*2]
				t3 := data[off+dist*3]

				// 执行蝶形运算
				t0, t1 = fwht2alt8(t0, t1)
				t2, t3 = fwht2alt8(t2, t3)
				t0, t2 = fwht2alt8(t0, t2)
				t1, t3 = fwht2alt8(t1, t3)

				// 写回结果
				data[off] = t0
				data[off+dist] = t1
				data[off+dist*2] = t2
				data[off+dist*3] = t3
				off++
			}
		}
		// 更新距离值
		dist = dist4
		dist4 <<= 2
	}
}

// fwht48 对4个元素执行FWHT变换
// 参数:
//   - data: 输入数据切片
//   - s: 步长
func fwht48(data []ffe8, s int) {
	// 计算两倍步长
	s2 := s << 1

	// 获取4个元素的指针
	t0 := &data[0]
	t1 := &data[s]
	t2 := &data[s2]
	t3 := &data[s2+s]

	// 执行蝶形运算
	fwht28(t0, t1)
	fwht28(t2, t3)
	fwht28(t0, t2)
	fwht28(t1, t3)
}

// fwht28 执行2点FWHT变换: {a, b} = {a + b, a - b} (模Q)
// 参数:
//   - a: 第一个元素的指针
//   - b: 第二个元素的指针
func fwht28(a, b *ffe8) {
	// 计算和与差
	sum := addMod8(*a, *b)
	dif := subMod8(*a, *b)
	// 更新结果
	*a = sum
	*b = dif
}

// fwht2alt8 与fwht28功能相同,但返回结果而不是修改输入
// 参数:
//   - a: 第一个输入值
//   - b: 第二个输入值
//
// 返回值:
//   - ffe8: a+b的结果
//   - ffe8: a-b的结果
func fwht2alt8(a, b ffe8) (ffe8, ffe8) {
	return addMod8(a, b), subMod8(a, b)
}

// initOnce8 确保初始化代码只执行一次
var initOnce8 sync.Once

// initConstants8 初始化所有常量表
// 使用sync.Once确保只初始化一次
func initConstants8() {
	initOnce8.Do(func() {
		initLUTs8()    // 初始化对数和指数查找表
		initFFTSkew8() // 初始化FFT倾斜因子
		initMul8LUT()  // 初始化乘法查找表
	})
}

// initLUTs8 初始化对数表logLUT8和指数表expLUT8
// 使用Cantor基生成有限域元素的对数和指数映射
func initLUTs8() {
	// 定义Cantor基向量
	cantorBasis := [bitwidth8]ffe8{
		1, 214, 152, 146, 86, 200, 88, 230,
	}

	// 分配查找表内存
	expLUT8 = &[order8]ffe8{}
	logLUT8 = &[order8]ffe8{}

	// 使用LFSR生成指数表
	state := 1
	for i := ffe8(0); i < modulus8; i++ {
		expLUT8[state] = i
		state <<= 1
		if state >= order8 {
			state ^= polynomial8
		}
	}
	expLUT8[0] = modulus8

	// 转换为Cantor基
	logLUT8[0] = 0
	for i := 0; i < bitwidth8; i++ {
		basis := cantorBasis[i]
		width := 1 << i

		// 生成对数表
		for j := 0; j < width; j++ {
			logLUT8[j+width] = logLUT8[j] ^ basis
		}
	}

	// 完成对数表到指数表的映射
	for i := 0; i < order8; i++ {
		logLUT8[i] = expLUT8[logLUT8[i]]
	}

	// 完成指数表到对数表的映射
	for i := 0; i < order8; i++ {
		expLUT8[logLUT8[i]] = ffe8(i)
	}

	expLUT8[modulus8] = expLUT8[0]
}

// initFFTSkew8 初始化FFT倾斜因子表fftSkew8和对数Walsh变换表logWalsh8
func initFFTSkew8() {
	// 临时存储中间计算结果
	var temp [bitwidth8 - 1]ffe8

	// 生成FFT倾斜向量的初始值
	for i := 1; i < bitwidth8; i++ {
		temp[i-1] = ffe8(1 << i)
	}

	// 分配查找表内存
	fftSkew8 = &[modulus8]ffe8{}
	logWalsh8 = &[order8]ffe8{}

	// 计算FFT倾斜因子
	for m := 0; m < bitwidth8-1; m++ {
		step := 1 << (m + 1)

		fftSkew8[1<<m-1] = 0

		// 计算每一层的倾斜因子
		for i := m; i < bitwidth8-1; i++ {
			s := 1 << (i + 1)

			for j := 1<<m - 1; j < s; j += step {
				fftSkew8[j+s] = fftSkew8[j] ^ temp[i]
			}
		}

		// 更新临时数组
		temp[m] = modulus8 - logLUT8[mulLog8(temp[m], logLUT8[temp[m]^1])]

		for i := m + 1; i < bitwidth8-1; i++ {
			sum := addMod8(logLUT8[temp[i]^1], temp[m])
			temp[i] = mulLog8(temp[i], sum)
		}
	}

	// 完成倾斜因子表的计算
	for i := 0; i < modulus8; i++ {
		fftSkew8[i] = logLUT8[fftSkew8[i]]
	}

	// 预计算对数值的Walsh变换
	for i := 0; i < order8; i++ {
		logWalsh8[i] = logLUT8[i]
	}
	logWalsh8[0] = 0

	fwht8(logWalsh8, order8)
}

// initMul8LUT 初始化乘法查找表
// 生成用于快速乘法运算的查找表
func initMul8LUT() {
	// 分配乘法查找表内存
	mul8LUTs = &[order8]mul8LUT{}

	// 为每个对数乘数生成查找表
	for log_m := 0; log_m < order8; log_m++ {
		var tmp [64]ffe8
		// 按4位nibble生成查找表
		for nibble, shift := 0, 0; nibble < 4; {
			nibble_lut := tmp[nibble*16:]

			for xnibble := 0; xnibble < 16; xnibble++ {
				prod := mulLog8(ffe8(xnibble<<shift), ffe8(log_m))
				nibble_lut[xnibble] = prod
			}
			nibble++
			shift += 4
		}

		// 组合查找表结果
		lut := &mul8LUTs[log_m]
		for i := range lut.Value[:] {
			lut.Value[i] = tmp[i&15] ^ tmp[((i>>4)+16)]
		}
	}

	// 初始化汇编优化的乘法表
	if true {
		multiply256LUT8 = &[order8][16 * 2]byte{}

		for logM := range multiply256LUT8[:] {
			shift := 0
			// 为每4位生成PSHUFB指令的查找表
			for i := 0; i < 2; i++ {
				prod := multiply256LUT8[logM][i*16 : i*16+16]
				for x := range prod[:] {
					prod[x] = byte(mulLog8(ffe8(x<<shift), ffe8(logM)))
				}
				shift += 4
			}
		}
	}
}

// kWords8 定义每个字段使用的64位字数
const kWords8 = order8 / 64

// errorBitfield8 包含用于指示哪些分片需要重建的渐进式错误位图
type errorBitfield8 struct {
	// Words 存储7个层级的错误位图,每个层级使用kWords8个uint64表示
	Words [7][kWords8]uint64
}

// set 在错误位图的第0层设置指定位置的错误标记
// i: 要设置错误标记的位置
func (e *errorBitfield8) set(i int) {
	e.Words[0][(i/64)&3] |= uint64(1) << (i & 63)
}

// cacheID 返回错误位图第0层的字节表示,用于缓存标识
// 返回值: 包含错误位图第0层数据的字节数组
func (e *errorBitfield8) cacheID() [inversion8Bytes]byte {
	var res [inversion8Bytes]byte
	// 将第0层的4个uint64转换为小端字节序
	binary.LittleEndian.PutUint64(res[0:8], e.Words[0][0])
	binary.LittleEndian.PutUint64(res[8:16], e.Words[0][1])
	binary.LittleEndian.PutUint64(res[16:24], e.Words[0][2])
	binary.LittleEndian.PutUint64(res[24:32], e.Words[0][3])
	return res
}

// isNeeded 检查指定mip层级和位置是否需要处理
// mipLevel: mip层级
// bit: 位置
// 返回值: 是否需要处理
func (e *errorBitfield8) isNeeded(mipLevel, bit int) bool {
	if mipLevel >= 8 || mipLevel <= 0 {
		return true
	}
	return 0 != (e.Words[mipLevel-1][bit/64] & (uint64(1) << (bit & 63)))
}

// prepare 准备错误位图的各个mip层级
// 通过位操作将第0层的错误标记传播到更高层级
func (e *errorBitfield8) prepare() {
	// 第一个mip层级用于FFT的最后一层:数据对
	for i := 0; i < kWords8; i++ {
		w_i := e.Words[0][i]
		// 向下传播高位到低位
		hi2lo0 := w_i | ((w_i & kHiMasks[0]) >> 1)
		// 向上传播低位到高位
		lo2hi0 := (w_i & (kHiMasks[0] >> 1)) << 1
		w_i = hi2lo0 | lo2hi0
		e.Words[0][i] = w_i

		bits := 2
		// 生成1-4层的mip图
		for j := 1; j < 5; j++ {
			hi2lo_j := w_i | ((w_i & kHiMasks[j]) >> bits)
			lo2hi_j := (w_i & (kHiMasks[j] >> bits)) << bits
			w_i = hi2lo_j | lo2hi_j
			e.Words[j][i] = w_i
			bits <<= 1
		}
	}

	// 生成第5层mip图
	for i := 0; i < kWords8; i++ {
		w := e.Words[4][i]
		w |= w >> 32
		w |= w << 32
		e.Words[5][i] = w
	}

	// 生成第6层mip图
	for i := 0; i < kWords8; i += 2 {
		t := e.Words[5][i] | e.Words[5][i+1]
		e.Words[6][i] = t
		e.Words[6][i+1] = t
	}
}

// fftDIT8 执行基8的快速傅里叶变换(FFT)的抽取时间(DIT)算法
// work: 输入/输出数据切片
// mtrunc: 截断长度
// m: 变换长度
// skewLUT: 倾斜因子查找表
// o: 选项参数
func (e *errorBitfield8) fftDIT8(work [][]byte, mtrunc, m int, skewLUT []ffe8, o *options) {
	// 计算最高mip层级
	mipLevel := bits.Len32(uint32(m)) - 1

	// 每次处理4个元素的距离
	dist4 := m
	dist := m >> 2
	// 每次展开2层FFT进行计算
	for dist != 0 {
		// 对每组dist*4个元素进行处理
		for r := 0; r < mtrunc; r += dist4 {
			if !e.isNeeded(mipLevel, r) {
				continue
			}
			iEnd := r + dist
			// 获取倾斜因子
			logM01 := skewLUT[iEnd-1]
			logM02 := skewLUT[iEnd+dist-1]
			logM23 := skewLUT[iEnd+dist*2-1]

			// 对每组dist个元素进行处理
			for i := r; i < iEnd; i++ {
				fftDIT48(
					work[i:],
					dist,
					logM01,
					logM23,
					logM02,
					o)
			}
		}
		dist4 = dist
		dist >>= 2
		mipLevel -= 2
	}

	// 如果还剩一层需要处理
	if dist4 == 2 {
		for r := 0; r < mtrunc; r += 2 {
			if !e.isNeeded(mipLevel, r) {
				continue
			}
			logM := skewLUT[r+1-1]

			if logM == modulus8 {
				sliceXor(work[r], work[r+1], o)
			} else {
				fftDIT28(work[r], work[r+1], logM, o)
			}
		}
	}
}
