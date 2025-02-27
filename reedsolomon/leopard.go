package reedsolomon

// 这是一个 O(n*log n) 的 Reed-Solomon 码实现,移植自 C++ 库 https://github.com/catid/leopard。
//
// 该实现基于论文:
//
// S.-J. Lin, T. Y. Al-Naffouri, Y. S. Han, 和 W.-H. Chung, "基于快速傅里叶变换的新型多项式基及其在里德所罗门纠删码中的应用" IEEE 信息理论汇刊, 第 6284-6299 页, 2016 年 11 月。

import (
	"bytes"
	"io"
	"math/bits"
	"sync"
	"unsafe"

	"github.com/klauspost/cpuid/v2"
)

// leopardFF16 类似于 reedSolomon,但支持超过 256 个分片。
type leopardFF16 struct {
	dataShards   int // 数据分片数量,不应修改。
	parityShards int // 校验分片数量,不应修改。
	totalShards  int // 总分片数量。计算得出,不应修改。

	workPool sync.Pool

	o options
}

// newFF16 类似于 New,但支持超过 256 个分片。
//
// 参数:
// - dataShards: int 数据分片数量
// - parityShards: int 校验分片数量
// - opt: 可选参数,可以传递WithConcurrentStreamReads(true)和WithConcurrentStreamWrites(true)来启用并发读取和写入。
// 返回:
// - *leopardFF16: 新的 leopardFF16 实例
func newFF16(dataShards, parityShards int, opt options) (*leopardFF16, error) {
	initConstants()

	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvShardNum
	}

	if dataShards+parityShards > 65536 {
		return nil, ErrMaxShardNum
	}

	r := &leopardFF16{
		dataShards:   dataShards,
		parityShards: parityShards,
		totalShards:  dataShards + parityShards,
		o:            opt,
	}
	return r, nil
}

var _ = Extensions(&leopardFF16{})

// ShardSizeMultiple 返回分片大小的倍数
func (r *leopardFF16) ShardSizeMultiple() int {
	return 64
}

// DataShards 返回数据分片数量
func (r *leopardFF16) DataShards() int {
	return r.dataShards
}

// ParityShards 返回校验分片数量
func (r *leopardFF16) ParityShards() int {
	return r.parityShards
}

// TotalShards 返回总分片数量
func (r *leopardFF16) TotalShards() int {
	return r.totalShards
}

// AllocAligned 分配一个对齐的切片
func (r *leopardFF16) AllocAligned(each int) [][]byte {
	return AllocAligned(r.totalShards, each)
}

type ffe uint16

const (
	bitwidth   = 16
	order      = 1 << bitwidth
	modulus    = order - 1
	polynomial = 0x1002D
)

var (
	fftSkew  *[modulus]ffe
	logWalsh *[order]ffe
)

// 对数表
var (
	logLUT *[order]ffe
	expLUT *[order]ffe
)

// 存储 x * y 在偏移量 x + y * 65536 处的部分积。从相同的 y 值重复访问更快
var mul16LUTs *[order]mul16LUT

type mul16LUT struct {
	// 包含作为单个查找的 Lo 乘积。
	// 应与 Hi 查找进行异或以获得结果。
	Lo [256]ffe
	Hi [256]ffe
}

// 存储 avx2 的查找表
var multiply256LUT *[order][8 * 16]byte

// Encode 编码数据分片
func (r *leopardFF16) Encode(shards [][]byte) error {
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if err := checkShards(shards, false); err != nil {
		return err
	}
	return r.encode(shards)
}

// encode 编码数据分片
func (r *leopardFF16) encode(shards [][]byte) error {
	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	m := ceilPow2(r.parityShards)
	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
	if cap(work) >= m*2 {
		work = work[:m*2]
	} else {
		work = AllocAligned(m*2, shardSize)
	}
	for i := range work {
		if cap(work[i]) < shardSize {
			work[i] = AllocAligned(1, shardSize)[0]
		} else {
			work[i] = work[i][:shardSize]
		}
	}
	defer r.workPool.Put(work)

	mtrunc := m
	if r.dataShards < mtrunc {
		mtrunc = r.dataShards
	}

	skewLUT := fftSkew[m-1:]

	sh := shards
	ifftDITEncoder(
		sh[:r.dataShards],
		mtrunc,
		work,
		nil, // 无异或输出
		m,
		skewLUT,
		&r.o,
	)

	lastCount := r.dataShards % m
	if m >= r.dataShards {
		goto skip_body
	}

	// 对于每组 m 个数据片:
	for i := m; i+m <= r.dataShards; i += m {
		sh = sh[m:]
		skewLUT = skewLUT[m:]

		// work <- work xor IFFT(data + i, m, m + i)

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

	// 处理最后一组不完整的 m 个片:
	if lastCount != 0 {
		sh = sh[m:]
		skewLUT = skewLUT[m:]

		// work <- work xor IFFT(data + i, m, m + i)

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
	// work <- FFT(work, m, 0)
	fftDIT(work, r.parityShards, m, fftSkew[:], &r.o)

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

// EncodeIdx 编码数据分片
func (r *leopardFF16) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	return ErrNotSupported
}

// Join 将分片连接起来并将数据段写入dst
func (r *leopardFF16) Join(dst io.Writer, shards [][]byte, outSize int) error {
	// 我们有足够的分片吗?
	if len(shards) < r.dataShards {
		return ErrTooFewShards
	}
	shards = shards[:r.dataShards]

	// 我们有足够的数据吗?
	size := 0
	for _, shard := range shards {
		if shard == nil {
			return ErrReconstructRequired
		}
		size += len(shard)

		// 我们已经有足够的数据了吗?
		if size >= outSize {
			break
		}
	}
	if size < outSize {
		return ErrShortData
	}

	// 将数据复制到 dst
	write := outSize
	for _, shard := range shards {
		if write < len(shard) {
			_, err := dst.Write(shard[:write])
			return err
		}
		n, err := dst.Write(shard)
		if err != nil {
			return err
		}
		write -= n
	}
	return nil
}

// Update 更新数据分片
func (r *leopardFF16) Update(shards [][]byte, newDatashards [][]byte) error {
	return ErrNotSupported
}

// Split 将数据分割成等长的分片
func (r *leopardFF16) Split(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, ErrShortData
	}
	if r.totalShards == 1 && len(data)&63 == 0 {
		return [][]byte{data}, nil
	}
	dataLen := len(data)
	// 计算每个数据分片的字节数。
	perShard := (len(data) + r.dataShards - 1) / r.dataShards
	perShard = ((perShard + 63) / 64) * 64
	needTotal := r.totalShards * perShard

	if cap(data) > len(data) {
		if cap(data) > needTotal {
			data = data[:needTotal]
		} else {
			data = data[:cap(data)]
		}
		clear := data[dataLen:]
		for i := range clear {
			clear[i] = 0
		}
	}

	// 仅在必要时分配内存
	var padding [][]byte
	if len(data) < needTotal {
		// 计算 `data` 切片中完整分片的最大数量
		fullShards := len(data) / perShard
		padding = AllocAligned(r.totalShards-fullShards, perShard)
		if dataLen > perShard*fullShards {
			// 复制部分分片
			copyFrom := data[perShard*fullShards : dataLen]
			for i := range padding {
				if len(copyFrom) == 0 {
					break
				}
				copyFrom = copyFrom[copy(padding[i], copyFrom):]
			}
		}
	} else {
		zero := data[dataLen : r.totalShards*perShard]
		for i := range zero {
			zero[i] = 0
		}
	}

	// 分割成等长的分片。
	dst := make([][]byte, r.totalShards)
	i := 0
	for ; i < len(dst) && len(data) >= perShard; i++ {
		dst[i] = data[:perShard:perShard]
		data = data[perShard:]
	}

	for j := 0; i+j < len(dst); j++ {
		dst[i+j] = padding[0]
		padding = padding[1:]
	}

	return dst, nil
}

// ReconstructSome 重建部分数据分片
func (r *leopardFF16) ReconstructSome(shards [][]byte, required []bool) error {
	if len(required) == r.totalShards {
		return r.reconstruct(shards, true)
	}
	return r.reconstruct(shards, false)
}

// Reconstruct 重建数据分片
func (r *leopardFF16) Reconstruct(shards [][]byte) error {
	return r.reconstruct(shards, true)
}

// ReconstructData 重建数据分片
func (r *leopardFF16) ReconstructData(shards [][]byte) error {
	return r.reconstruct(shards, false)
}

// Verify 验证数据分片
func (r *leopardFF16) Verify(shards [][]byte) (bool, error) {
	if len(shards) != r.totalShards {
		return false, ErrTooFewShards
	}
	if err := checkShards(shards, false); err != nil {
		return false, err
	}

	// 将校验分片重新编码到临时存储。
	shardSize := len(shards[0])
	outputs := make([][]byte, r.totalShards)
	copy(outputs, shards[:r.dataShards])
	for i := r.dataShards; i < r.totalShards; i++ {
		outputs[i] = make([]byte, shardSize)
	}
	if err := r.Encode(outputs); err != nil {
		return false, err
	}

	// 比较。
	for i := r.dataShards; i < r.totalShards; i++ {
		if !bytes.Equal(outputs[i], shards[i]) {
			return false, nil
		}
	}
	return true, nil
}

// reconstruct 重建数据分片
func (r *leopardFF16) reconstruct(shards [][]byte, recoverAll bool) error {
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if err := checkShards(shards, true); err != nil {
		return err
	}

	// 快速检查:所有分片都存在吗? 如果是,就没有什么要做的。
	numberPresent := 0
	dataPresent := 0
	for i := 0; i < r.totalShards; i++ {
		if len(shards[i]) != 0 {
			numberPresent++
			if i < r.dataShards {
				dataPresent++
			}
		}
	}
	if numberPresent == r.totalShards || !recoverAll && dataPresent == r.dataShards {
		// 很好。所有分片都有数据。我们不需要做任何事。
		return nil
	}

	// 仅在丢失的校验分片少于 1/4 时使用。
	useBits := r.totalShards-numberPresent <= r.parityShards/4

	// 检查我们是否有足够的分片进行重建。
	if numberPresent < r.dataShards {
		return ErrTooFewShards
	}

	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	m := ceilPow2(r.parityShards)
	n := ceilPow2(m + r.dataShards)

	const LEO_ERROR_BITFIELD_OPT = true

	// 填充错误位置。
	var errorBits errorBitfield
	var errLocs [order]ffe
	for i := 0; i < r.parityShards; i++ {
		if len(shards[i+r.dataShards]) == 0 {
			errLocs[i] = 1
			if LEO_ERROR_BITFIELD_OPT && recoverAll {
				errorBits.set(i)
			}
		}
	}
	for i := r.parityShards; i < m; i++ {
		errLocs[i] = 1
		if LEO_ERROR_BITFIELD_OPT && recoverAll {
			errorBits.set(i)
		}
	}
	for i := 0; i < r.dataShards; i++ {
		if len(shards[i]) == 0 {
			errLocs[i+m] = 1
			if LEO_ERROR_BITFIELD_OPT {
				errorBits.set(i + m)
			}
		}
	}

	if LEO_ERROR_BITFIELD_OPT && useBits {
		errorBits.prepare()
	}

	// 评估错误定位多项式
	fwht(&errLocs, m+r.dataShards)

	for i := 0; i < order; i++ {
		errLocs[i] = ffe((uint(errLocs[i]) * uint(logWalsh[i])) % modulus)
	}

	fwht(&errLocs, order)

	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
	if cap(work) >= n {
		work = work[:n]
	} else {
		work = make([][]byte, n)
	}
	for i := range work {
		if cap(work[i]) < shardSize {
			work[i] = make([]byte, shardSize)
		} else {
			work[i] = work[i][:shardSize]
		}
	}
	defer r.workPool.Put(work)

	// work <- 恢复数据

	for i := 0; i < r.parityShards; i++ {
		if len(shards[i+r.dataShards]) != 0 {
			mulgf16(work[i], shards[i+r.dataShards], errLocs[i], &r.o)
		} else {
			memclr(work[i])
		}
	}
	for i := r.parityShards; i < m; i++ {
		memclr(work[i])
	}

	// work <- 原始数据

	for i := 0; i < r.dataShards; i++ {
		if len(shards[i]) != 0 {
			mulgf16(work[m+i], shards[i], errLocs[m+i], &r.o)
		} else {
			memclr(work[m+i])
		}
	}
	for i := m + r.dataShards; i < n; i++ {
		memclr(work[i])
	}

	// work <- IFFT(work, n, 0)

	ifftDITDecoder(
		m+r.dataShards,
		work,
		n,
		fftSkew[:],
		&r.o,
	)

	// work <- FormalDerivative(work, n)

	for i := 1; i < n; i++ {
		width := ((i ^ (i - 1)) + 1) >> 1
		slicesXor(work[i-width:i], work[i:i+width], &r.o)
	}

	// work <- FFT(work, n, 0) 截断到 m + dataShards

	outputCount := m + r.dataShards

	if LEO_ERROR_BITFIELD_OPT && useBits {
		errorBits.fftDIT(work, outputCount, n, fftSkew[:], &r.o)
	} else {
		fftDIT(work, outputCount, n, fftSkew[:], &r.o)
	}

	// 揭示擦除
	//
	//  Original = -ErrLocator * FFT( Derivative( IFFT( ErrLocator * ReceivedData ) ) )
	//  mul_mem(x, y, log_m, ) 等于 x[] = y[] * log_m
	//
	// 内存布局: [恢复数据 (2的幂 = M)] [原始数据 (K)] [零填充到 N]
	end := r.dataShards
	if recoverAll {
		end = r.totalShards
	}
	for i := 0; i < end; i++ {
		if len(shards[i]) != 0 {
			continue
		}
		if cap(shards[i]) >= shardSize {
			shards[i] = shards[i][:shardSize]
		} else {
			shards[i] = make([]byte, shardSize)
		}
		if i >= r.dataShards {
			// 校验分片。
			mulgf16(shards[i], work[i-r.dataShards], modulus-errLocs[i-r.dataShards], &r.o)
		} else {
			// 数据分片。
			mulgf16(shards[i], work[i+m], modulus-errLocs[i+m], &r.o)
		}
	}
	return nil
}

// ifftDITDecoder 解码器的基本无修饰版
func ifftDITDecoder(mtrunc int, work [][]byte, m int, skewLUT []ffe, o *options) {
	// 时域抽取:每次展开 2 层
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// 对于每组 dist*4 个元素:
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend-1]
			log_m02 := skewLUT[iend+dist-1]
			log_m23 := skewLUT[iend+dist*2-1]

			// 对于每组 dist 个元素:
			for i := r; i < iend; i++ {
				ifftDIT4(work[i:], dist, log_m01, log_m23, log_m02, o)
			}
		}
		dist = dist4
		dist4 <<= 2
	}

	// 如果还剩一层:
	if dist < m {
		// 假设 dist = m / 2
		if dist*2 != m {
			panic("内部错误")
		}

		log_m := skewLUT[dist-1]

		if log_m == modulus {
			slicesXor(work[dist:2*dist], work[:dist], o)
		} else {
			for i := 0; i < dist; i++ {
				ifftDIT2(
					work[i],
					work[i+dist],
					log_m,
					o,
				)
			}
		}
	}
}

// fftDIT 编码器和解码器的就地 FFT
func fftDIT(work [][]byte, mtrunc, m int, skewLUT []ffe, o *options) {
	// 时域抽取:每次展开 2 层
	dist4 := m
	dist := m >> 2
	for dist != 0 {
		// 对于每组 dist*4 个元素:
		for r := 0; r < mtrunc; r += dist4 {
			iEnd := r + dist
			logM01 := skewLUT[iEnd-1]
			logM02 := skewLUT[iEnd+dist-1]
			logM23 := skewLUT[iEnd+dist*2-1]

			// 对于每组 dist 个元素:
			for i := r; i < iEnd; i++ {
				fftDIT4(
					work[i:],
					dist,
					logM01,
					logM23,
					logM02,
					o,
				)
			}
		}
		dist4 = dist
		dist >>= 2
	}

	// 如果还剩一层:
	if dist4 == 2 {
		for r := 0; r < mtrunc; r += 2 {
			logM := skewLUT[r+1-1]

			if logM == modulus {
				sliceXor(work[r], work[r+1], o)
			} else {
				fftDIT2(work[r], work[r+1], logM, o)
			}
		}
	}
}

// fftDIT4Ref 4 路蝶形运算
func fftDIT4Ref(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	// 第一层:
	if log_m02 == modulus {
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		fftDIT2(work[0], work[dist*2], log_m02, o)
		fftDIT2(work[dist], work[dist*3], log_m02, o)
	}

	// 第二层:
	if log_m01 == modulus {
		sliceXor(work[0], work[dist], o)
	} else {
		fftDIT2(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus {
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		fftDIT2(work[dist*2], work[dist*3], log_m23, o)
	}
}

// ifftDITEncoder 编码器的展开 IFFT
func ifftDITEncoder(data [][]byte, mtrunc int, work [][]byte, xorRes [][]byte, m int, skewLUT []ffe, o *options) {
	// 我尝试将 memcpy/memset 合并到 FFT 的第一层中,发现它只能提供 4% 的性能改进,这不值得增加额外的复杂性。
	for i := 0; i < mtrunc; i++ {
		copy(work[i], data[i])
	}
	for i := mtrunc; i < m; i++ {
		memclr(work[i])
	}

	// 我尝试将前几层分成 L3 缓存大小的块,但发现它只能提供约 5% 的性能提升,这不值得增加额外的复杂性。

	// 时域抽取:每次展开 2 层
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// 对于每组 dist*4 个元素:
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend]
			log_m02 := skewLUT[iend+dist]
			log_m23 := skewLUT[iend+dist*2]

			// 对于每组 dist 个元素:
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

		dist = dist4
		dist4 <<= 2
		// 我尝试交替从左到右和从右到左扫描以减少缓存未命中。
		// 当对 FFT 和 IFFT 都这样做时,它只提供约 1% 的性能提升,所以似乎不值得增加额外的复杂性。
	}

	// 如果还剩一层:
	if dist < m {
		// 假设 dist = m / 2
		if dist*2 != m {
			panic("内部错误")
		}

		logm := skewLUT[dist]

		if logm == modulus {
			slicesXor(work[dist:dist*2], work[:dist], o)
		} else {
			for i := 0; i < dist; i++ {
				ifftDIT2(work[i], work[i+dist], logm, o)
			}
		}
	}

	// 我尝试展开这个但它对于 16 位有限域来说不能提供超过 5% 的性能改进,所以不值得增加复杂性。
	if xorRes != nil {
		slicesXor(xorRes[:m], work[:m], o)
	}
}

// ifftDIT4Ref 4 路蝶形运算
func ifftDIT4Ref(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	// 第一层:
	if log_m01 == modulus {
		sliceXor(work[0], work[dist], o)
	} else {
		ifftDIT2(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus {
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		ifftDIT2(work[dist*2], work[dist*3], log_m23, o)
	}

	// 第二层:
	if log_m02 == modulus {
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		ifftDIT2(work[0], work[dist*2], log_m02, o)
		ifftDIT2(work[dist], work[dist*3], log_m02, o)
	}
}

// refMulAdd muladd 的参考版本: x[] ^= y[] * log_m
func refMulAdd(x, y []byte, log_m ffe) {
	lut := &mul16LUTs[log_m]

	for len(x) >= 64 {
		// 断言大小以避免循环中的边界检查
		hiA := y[32:64]
		loA := y[:32]
		dst := x[:64] // 需要但不检查...
		for i, lo := range loA {
			hi := hiA[i]
			prod := lut.Lo[lo] ^ lut.Hi[hi]

			dst[i] ^= byte(prod)
			dst[i+32] ^= byte(prod >> 8)
		}
		x = x[64:]
		y = y[64:]
	}
}

// memclr 将 s 中的所有字节设置为 0
func memclr(s []byte) {
	for i := range s {
		s[i] = 0
	}
}

// slicesXor 对 v1, v2 中的每对切片调用 xor。
func slicesXor(v1, v2 [][]byte, o *options) {
	for i, v := range v1 {
		sliceXor(v2[i], v, o)
	}
}

// refMul mul 的参考版本: x[] = y[] * log_m
func refMul(x, y []byte, log_m ffe) {
	lut := &mul16LUTs[log_m]

	for off := 0; off < len(x); off += 64 {
		loA := y[off : off+32]
		hiA := y[off+32:]
		hiA = hiA[:len(loA)]
		for i, lo := range loA {
			hi := hiA[i]
			prod := lut.Lo[lo] ^ lut.Hi[hi]

			x[off+i] = byte(prod)
			x[off+i+32] = byte(prod >> 8)
		}
	}
}

// 返回 a * Log(b)
func mulLog(a, log_b ffe) ffe {
	/*
	   注意这不是有限域中的普通乘法,因为右操作数已经是对数。
	   这样做是因为它将 K 次表查找从性能关键的 Decode() 方法移到了初始化步骤中。
	   LogWalsh[] 表包含预计算的对数,所以用这种形式做其他乘法也更容易。
	*/
	if a == 0 {
		return 0
	}
	return expLUT[addMod(logLUT[a], log_b)]
}

// z = x + y (mod kModulus)
func addMod(a, b ffe) ffe {
	sum := uint(a) + uint(b)

	// 部分约简步骤,允许返回 kModulus
	return ffe(sum + sum>>bitwidth)
}

// z = x - y (mod kModulus)
func subMod(a, b ffe) ffe {
	dif := uint(a) - uint(b)

	// 部分约简步骤,允许返回 kModulus
	return ffe(dif + dif>>bitwidth)
}

// ceilPow2 返回大于或等于 n 的 2 的幂。
func ceilPow2(n int) int {
	const w = int(unsafe.Sizeof(n) * 8)
	return 1 << (w - bits.LeadingZeros(uint(n-1)))
}

// 时域抽取(DIT)快速沃尔什-阿达玛变换
// 展开成对层以在寄存器中执行跨层操作
// mtrunc: 数据前端非零元素的数量
func fwht(data *[order]ffe, mtrunc int) {
	// 时域抽取:每次展开 2 层
	dist := 1
	dist4 := 4
	for dist4 <= order {
		// 对于每组 dist*4 个元素:
		for r := 0; r < mtrunc; r += dist4 {
			// 对于每组 dist 个元素:
			// 使用 16 位索引以避免 [65536]ffe 的边界检查
			dist := uint16(dist)
			off := uint16(r)
			for i := uint16(0); i < dist; i++ {
				// 内联 fwht4(data[i:], dist)...
				// 读取值似乎比更新指针更快
				// 转换为 uint 并不会更快
				t0 := data[off]
				t1 := data[off+dist]
				t2 := data[off+dist*2]
				t3 := data[off+dist*3]

				t0, t1 = fwht2alt(t0, t1)
				t2, t3 = fwht2alt(t2, t3)
				t0, t2 = fwht2alt(t0, t2)
				t1, t3 = fwht2alt(t1, t3)

				data[off] = t0
				data[off+dist] = t1
				data[off+dist*2] = t2
				data[off+dist*3] = t3
				off++
			}
		}
		dist = dist4
		dist4 <<= 2
	}
}

func fwht4(data []ffe, s int) {
	s2 := s << 1

	t0 := &data[0]
	t1 := &data[s]
	t2 := &data[s2]
	t3 := &data[s2+s]

	fwht2(t0, t1)
	fwht2(t2, t3)
	fwht2(t0, t2)
	fwht2(t1, t3)
}

// {a, b} = {a + b, a - b} (模 Q)
func fwht2(a, b *ffe) {
	sum := addMod(*a, *b)
	dif := subMod(*a, *b)
	*a = sum
	*b = dif
}

// fwht2alt 与 fwht2 相同,但返回结果
func fwht2alt(a, b ffe) (ffe, ffe) {
	return addMod(a, b), subMod(a, b)
}

var initOnce sync.Once

func initConstants() {
	initOnce.Do(func() {
		initLUTs()
		initFFTSkew()
		initMul16LUT()
	})
}

// 初始化 logLUT, expLUT
func initLUTs() {
	cantorBasis := [bitwidth]ffe{
		0x0001, 0xACCA, 0x3C0E, 0x163E,
		0xC582, 0xED2E, 0x914C, 0x4012,
		0x6C98, 0x10D8, 0x6A72, 0xB900,
		0xFDB8, 0xFB34, 0xFF38, 0x991E,
	}

	expLUT = &[order]ffe{}
	logLUT = &[order]ffe{}

	// 生成 LFSR 表:
	state := 1
	for i := ffe(0); i < modulus; i++ {
		expLUT[state] = i
		state <<= 1
		if state >= order {
			state ^= polynomial
		}
	}
	expLUT[0] = modulus

	// 转换为 Cantor 基:

	logLUT[0] = 0
	for i := 0; i < bitwidth; i++ {
		basis := cantorBasis[i]
		width := 1 << i

		for j := 0; j < width; j++ {
			logLUT[j+width] = logLUT[j] ^ basis
		}
	}

	for i := 0; i < order; i++ {
		logLUT[i] = expLUT[logLUT[i]]
	}

	for i := 0; i < order; i++ {
		expLUT[logLUT[i]] = ffe(i)
	}

	expLUT[modulus] = expLUT[0]
}

// 初始化 fftSkew
func initFFTSkew() {
	var temp [bitwidth - 1]ffe

	// 生成 FFT 偏移向量 {1}:

	for i := 1; i < bitwidth; i++ {
		temp[i-1] = ffe(1 << i)
	}

	fftSkew = &[modulus]ffe{}
	logWalsh = &[order]ffe{}

	for m := 0; m < bitwidth-1; m++ {
		step := 1 << (m + 1)

		fftSkew[1<<m-1] = 0

		for i := m; i < bitwidth-1; i++ {
			s := 1 << (i + 1)

			for j := 1<<m - 1; j < s; j += step {
				fftSkew[j+s] = fftSkew[j] ^ temp[i]
			}
		}

		temp[m] = modulus - logLUT[mulLog(temp[m], logLUT[temp[m]^1])]

		for i := m + 1; i < bitwidth-1; i++ {
			sum := addMod(logLUT[temp[i]^1], temp[m])
			temp[i] = mulLog(temp[i], sum)
		}
	}

	for i := 0; i < modulus; i++ {
		fftSkew[i] = logLUT[fftSkew[i]]
	}

	// 预计算 FWHT(Log[i]):

	for i := 0; i < order; i++ {
		logWalsh[i] = logLUT[i]
	}
	logWalsh[0] = 0

	fwht(logWalsh, order)
}

func initMul16LUT() {
	mul16LUTs = &[order]mul16LUT{}

	// 对于每个 log_m 乘数:
	for log_m := 0; log_m < order; log_m++ {
		var tmp [64]ffe
		for nibble, shift := 0, 0; nibble < 4; {
			nibble_lut := tmp[nibble*16:]

			for xnibble := 0; xnibble < 16; xnibble++ {
				prod := mulLog(ffe(xnibble<<shift), ffe(log_m))
				nibble_lut[xnibble] = prod
			}
			nibble++
			shift += 4
		}
		lut := &mul16LUTs[log_m]
		for i := range lut.Lo[:] {
			lut.Lo[i] = tmp[i&15] ^ tmp[((i>>4)+16)]
			lut.Hi[i] = tmp[((i&15)+32)] ^ tmp[((i>>4)+48)]
		}
	}
	if cpuid.CPU.Has(cpuid.SSSE3) || cpuid.CPU.Has(cpuid.AVX2) || cpuid.CPU.Has(cpuid.AVX512F) {
		multiply256LUT = &[order][16 * 8]byte{}

		for logM := range multiply256LUT[:] {
			// 对于有限域位宽的每 4 位:
			shift := 0
			for i := 0; i < 4; i++ {
				// 构造用于 PSHUFB 的 16 项查找表
				prodLo := multiply256LUT[logM][i*16 : i*16+16]
				prodHi := multiply256LUT[logM][4*16+i*16 : 4*16+i*16+16]
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

const kWordMips = 5
const kWords = order / 64
const kBigMips = 6
const kBigWords = (kWords + 63) / 64
const kBiggestMips = 4

// errorBitfield 包含渐进错误,用于指示哪些分片需要重建
type errorBitfield struct {
	Words        [kWordMips][kWords]uint64
	BigWords     [kBigMips][kBigWords]uint64
	BiggestWords [kBiggestMips]uint64
}

func (e *errorBitfield) set(i int) {
	e.Words[0][i/64] |= uint64(1) << (i & 63)
}

func (e *errorBitfield) isNeededFn(mipLevel int) func(bit int) bool {
	if mipLevel >= 16 {
		return func(bit int) bool {
			return true
		}
	}
	if mipLevel >= 12 {
		w := e.BiggestWords[mipLevel-12]
		return func(bit int) bool {
			bit /= 4096
			return 0 != (w & (uint64(1) << bit))
		}
	}
	if mipLevel >= 6 {
		w := e.BigWords[mipLevel-6][:]
		return func(bit int) bool {
			bit /= 64
			return 0 != (w[bit/64] & (uint64(1) << (bit & 63)))
		}
	}
	if mipLevel > 0 {
		w := e.Words[mipLevel-1][:]
		return func(bit int) bool {
			return 0 != (w[bit/64] & (uint64(1) << (bit & 63)))
		}
	}
	return nil
}

func (e *errorBitfield) isNeeded(mipLevel int, bit uint) bool {
	if mipLevel >= 16 {
		return true
	}
	if mipLevel >= 12 {
		bit /= 4096
		return 0 != (e.BiggestWords[mipLevel-12] & (uint64(1) << bit))
	}
	if mipLevel >= 6 {
		bit /= 64
		return 0 != (e.BigWords[mipLevel-6][bit/64] & (uint64(1) << (bit % 64)))
	}
	return 0 != (e.Words[mipLevel-1][bit/64] & (uint64(1) << (bit % 64)))
}

var kHiMasks = [5]uint64{
	0xAAAAAAAAAAAAAAAA,
	0xCCCCCCCCCCCCCCCC,
	0xF0F0F0F0F0F0F0F0,
	0xFF00FF00FF00FF00,
	0xFFFF0000FFFF0000,
}

func (e *errorBitfield) prepare() {
	// 第一个 mip 级别用于 FFT 的最后一层:数据对
	for i := 0; i < kWords; i++ {
		w_i := e.Words[0][i]
		hi2lo0 := w_i | ((w_i & kHiMasks[0]) >> 1)
		lo2hi0 := (w_i & (kHiMasks[0] >> 1)) << 1
		w_i = hi2lo0 | lo2hi0
		e.Words[0][i] = w_i

		bits := 2
		for j := 1; j < kWordMips; j++ {
			hi2lo_j := w_i | ((w_i & kHiMasks[j]) >> bits)
			lo2hi_j := (w_i & (kHiMasks[j] >> bits)) << bits
			w_i = hi2lo_j | lo2hi_j
			e.Words[j][i] = w_i
			bits <<= 1
		}
	}

	for i := 0; i < kBigWords; i++ {
		w_i := uint64(0)
		bit := uint64(1)
		src := e.Words[kWordMips-1][i*64 : i*64+64]
		for _, w := range src {
			w_i |= (w | (w >> 32) | (w << 32)) & bit
			bit <<= 1
		}
		e.BigWords[0][i] = w_i

		bits := 1
		for j := 1; j < kBigMips; j++ {
			hi2lo_j := w_i | ((w_i & kHiMasks[j-1]) >> bits)
			lo2hi_j := (w_i & (kHiMasks[j-1] >> bits)) << bits
			w_i = hi2lo_j | lo2hi_j
			e.BigWords[j][i] = w_i
			bits <<= 1
		}
	}

	w_i := uint64(0)
	bit := uint64(1)
	for _, w := range e.BigWords[kBigMips-1][:kBigWords] {
		w_i |= (w | (w >> 32) | (w << 32)) & bit
		bit <<= 1
	}
	e.BiggestWords[0] = w_i

	bits := uint64(1)
	for j := 1; j < kBiggestMips; j++ {
		hi2lo_j := w_i | ((w_i & kHiMasks[j-1]) >> bits)
		lo2hi_j := (w_i & (kHiMasks[j-1] >> bits)) << bits
		w_i = hi2lo_j | lo2hi_j
		e.BiggestWords[j] = w_i
		bits <<= 1
	}
}

func (e *errorBitfield) fftDIT(work [][]byte, mtrunc, m int, skewLUT []ffe, o *options) {
	// 时域抽取:每次展开 2 层
	mipLevel := bits.Len32(uint32(m)) - 1

	dist4 := m
	dist := m >> 2
	needed := e.isNeededFn(mipLevel)
	for dist != 0 {
		// 对于每组 dist*4 个元素:
		for r := 0; r < mtrunc; r += dist4 {
			if !needed(r) {
				continue
			}
			iEnd := r + dist
			logM01 := skewLUT[iEnd-1]
			logM02 := skewLUT[iEnd+dist-1]
			logM23 := skewLUT[iEnd+dist*2-1]

			// 对于每组 dist 个元素:
			for i := r; i < iEnd; i++ {
				fftDIT4(
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
		needed = e.isNeededFn(mipLevel)
	}

	// 如果还剩一层:
	if dist4 == 2 {
		for r := 0; r < mtrunc; r += 2 {
			if !needed(r) {
				continue
			}
			logM := skewLUT[r+1-1]

			if logM == modulus {
				sliceXor(work[r], work[r+1], o)
			} else {
				fftDIT2(work[r], work[r+1], logM, o)
			}
		}
	}
}
