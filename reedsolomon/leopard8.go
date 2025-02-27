package reedsolomon

// 这是一个 O(n*log n) 的 Reed-Solomon 码实现,移植自 C++ 库 https://github.com/catid/leopard。
//
// 该实现基于论文:
//
// S.-J. Lin, T. Y. Al-Naffouri, Y. S. Han, 和 W.-H. Chung, "基于快速傅里叶变换的新型多项式基及其在里德所罗门纠删码中的应用" IEEE 信息理论汇刊, 第 6284-6299 页, 2016 年 11 月。
//
// S.-J. Lin、T. Y. Al-Naffouri、Y. S. Han 和 W.-H.
// Chung 所著《基于快速傅里叶变换的新型多项式基及其在里德所罗门纠删码中的应用》
// IEEE 信息理论汇刊,第 6284-6299 页,2016 年 11 月。

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/bits"
	"sync"
)

// leopardFF8 类似于 reedSolomon 但用于8位 "leopard" 实现。
type leopardFF8 struct {
	dataShards   int // 数据分片数量,不应修改。
	parityShards int // 校验分片数量,不应修改。
	totalShards  int // 总分片数量。计算得出,不应修改。

	workPool    sync.Pool
	inversion   map[[inversion8Bytes]byte]leopardGF8cache
	inversionMu sync.Mutex

	o options
}

// inversion8Bytes 用于存储纠错信息。
const inversion8Bytes = 256 / 8

// leopardGF8cache 用于存储纠错信息。
type leopardGF8cache struct {
	errorLocs [256]ffe8
	bits      *errorBitfield8
}

// newFF8 类似于 New,但用于8位 "leopard" 实现。
//
// 参数:
// - dataShards: 数据分片数量,必须大于0。
// - parityShards: 校验分片数量,必须大于0。
// - opt: 选项,用于配置纠错信息。
//
// 返回值:
// - *leopardFF8: 返回一个 leopardFF8 实例。
// - error: 如果参数无效,返回错误。
func newFF8(dataShards, parityShards int, opt options) (*leopardFF8, error) {
	initConstants8()

	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvShardNum
	}

	if dataShards+parityShards > 65536 {
		return nil, ErrMaxShardNum
	}

	r := &leopardFF8{
		dataShards:   dataShards,
		parityShards: parityShards,
		totalShards:  dataShards + parityShards,
		o:            opt,
	}
	if opt.inversionCache && (r.totalShards <= 64 || opt.forcedInversionCache) {
		// 对于大量分片数量来说,反转缓存的效果相对较差,并且可能占用大量内存。
		// r.totalShards 并不是实际占用的空间,而只是一个估计值。
		r.inversion = make(map[[inversion8Bytes]byte]leopardGF8cache, r.totalShards)
	}
	return r, nil
}

// ShardSizeMultiple 返回64。
var _ = Extensions(&leopardFF8{})

// ShardSizeMultiple 返回64。
func (r *leopardFF8) ShardSizeMultiple() int {
	return 64
}

// DataShards 返回数据分片数量。
func (r *leopardFF8) DataShards() int {
	return r.dataShards
}

// ParityShards 返回奇偶校验分片数量。
func (r *leopardFF8) ParityShards() int {
	return r.parityShards
}

// TotalShards 返回总分片数量。
func (r *leopardFF8) TotalShards() int {
	return r.totalShards
}

// AllocAligned 返回一个分配的内存。
func (r *leopardFF8) AllocAligned(each int) [][]byte {
	return AllocAligned(r.totalShards, each)
}

// ffe8 是一个8位的无符号整数。
type ffe8 uint8

const (
	bitwidth8   = 8              // 8位无符号整数的位宽
	order8      = 1 << bitwidth8 // 8位无符号整数的阶
	modulus8    = order8 - 1     // 模数
	polynomial8 = 0x11D          // 多项式

	// 以这个大小块编码。
	workSize8 = 32 << 10 // 32KB
)

var (
	fftSkew8  *[modulus8]ffe8 // 快速傅里叶变换的偏移量
	logWalsh8 *[order8]ffe8   // 对数沃尔什变换
)

// 对数表
var (
	logLUT8 *[order8]ffe8 // 对数查找表
	expLUT8 *[order8]ffe8 // 指数查找表
)

// 存储x * y的偏积,x + y * 256
// 重复访问相同的y值更快
var mul8LUTs *[order8]mul8LUT

// mul8LUT 存储x * y的偏积,x + y * 256
// 重复访问相同的y值更快
type mul8LUT struct {
	Value [256]ffe8
}

// 存储avx2的查找表
var multiply256LUT8 *[order8][2 * 16]byte

// Encode 编码shards。
func (r *leopardFF8) Encode(shards [][]byte) error {
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if err := checkShards(shards, false); err != nil {
		return err
	}
	return r.encode(shards)
}

// encode 编码shards。
func (r *leopardFF8) encode(shards [][]byte) error {
	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	m := ceilPow2(r.parityShards)
	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	} else {
		work = AllocAligned(m*2, workSize8)
	}
	if cap(work) >= m*2 {
		work = work[:m*2]
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
		work = AllocAligned(m*2, workSize8)
	}

	defer r.workPool.Put(work)

	mtrunc := m
	if r.dataShards < mtrunc {
		mtrunc = r.dataShards
	}

	skewLUT := fftSkew8[m-1:]

	// 分割大分片
	// 在较低的分片数量上更有可能。
	off := 0
	sh := make([][]byte, len(shards))

	// 我们可以修改的工作切片
	wMod := make([][]byte, len(work))
	copy(wMod, work)
	for off < shardSize {
		work := wMod
		sh := sh
		end := off + workSize8
		if end > shardSize {
			end = shardSize
			sz := shardSize - off
			for i := range work {
				// 最后一次迭代...
				work[i] = work[i][:sz]
			}
		}
		for i := range shards {
			sh[i] = shards[i][off:end]
		}

		// 替换工作切片,这样我们可以直接写入输出。
		// 注意work中的奇偶校验分片在数据分片之前。
		res := shards[r.dataShards:r.totalShards]
		for i := range res {
			work[i] = res[i][off:end]
		}

		ifftDITEncoder8(
			sh[:r.dataShards],
			mtrunc,
			work,
			nil, // 没有xor输出
			m,
			skewLUT,
			&r.o,
		)

		lastCount := r.dataShards % m
		skewLUT2 := skewLUT
		if m >= r.dataShards {
			goto skip_body
		}

		// 对于m个数据分片:
		for i := m; i+m <= r.dataShards; i += m {
			sh = sh[m:]
			skewLUT2 = skewLUT2[m:]

			// work <- work xor IFFT(data + i, m, m + i)

			ifftDITEncoder8(
				sh, // 数据源
				m,
				work[m:], // 临时工作空间
				work,     // xor目标
				m,
				skewLUT2,
				&r.o,
			)
		}

		// 处理最后一部分m个数据分片:
		if lastCount != 0 {
			sh = sh[m:]
			skewLUT2 = skewLUT2[m:]

			// work <- work xor IFFT(data + i, m, m + i)

			ifftDITEncoder8(
				sh, // 数据源
				lastCount,
				work[m:], // 临时工作空间
				work,     // xor目标
				m,
				skewLUT2,
				&r.o,
			)
		}

	skip_body:
		// work <- FFT(work, m, 0)
		fftDIT8(work, r.parityShards, m, fftSkew8[:], &r.o)
		off += workSize8
	}

	return nil
}

// EncodeIdx 编码单个数据分片。
func (r *leopardFF8) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	return ErrNotSupported
}

// Join 将shards连接到dst。
func (r *leopardFF8) Join(dst io.Writer, shards [][]byte, outSize int) error {
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

		// 我们有足够的数据了吗?
		if size >= outSize {
			break
		}
	}
	if size < outSize {
		return ErrShortData
	}

	// 将数据复制到dst
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

// Update 更新shards。
func (r *leopardFF8) Update(shards [][]byte, newDatashards [][]byte) error {
	return ErrNotSupported
}

// Split 将数据分割成shards。
func (r *leopardFF8) Split(data []byte) ([][]byte, error) {
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

	// 只有在必要时才分配内存
	var padding [][]byte
	if len(data) < needTotal {
		// 计算`data`切片中最多有多少个完整的数据分片
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
	}

	// 将数据分割成等长的分片。
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

// ReconstructSome 重建一些分片。
func (r *leopardFF8) ReconstructSome(shards [][]byte, required []bool) error {
	if len(required) == r.totalShards {
		return r.reconstruct(shards, true)
	}
	return r.reconstruct(shards, false)
}

// Reconstruct 重建所有分片。
func (r *leopardFF8) Reconstruct(shards [][]byte) error {
	return r.reconstruct(shards, true)
}

// ReconstructData 重建数据分片。
func (r *leopardFF8) ReconstructData(shards [][]byte) error {
	return r.reconstruct(shards, false)
}

// Verify 验证shards。
func (r *leopardFF8) Verify(shards [][]byte) (bool, error) {
	if len(shards) != r.totalShards {
		return false, ErrTooFewShards
	}
	if err := checkShards(shards, false); err != nil {
		return false, err
	}

	// 重新编码奇偶校验分片到临时存储。
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

// reconstruct 重建shards。
func (r *leopardFF8) reconstruct(shards [][]byte, recoverAll bool) error {
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if err := checkShards(shards, true); err != nil {
		return err
	}

	// 快速检查:所有分片是否存在? 如果是这样,就没有什么可做的了。
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
		// 很棒。所有分片都有数据。我们不需要做任何事情。
		return nil
	}

	// 检查我们是否有足够的分片来重建。
	if numberPresent < r.dataShards {
		return ErrTooFewShards
	}

	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	// 仅在缺失少于1/4奇偶校验分片且恢复大量数据时使用。
	useBits := r.totalShards-numberPresent <= r.parityShards/4 && shardSize*r.totalShards >= 64<<10

	m := ceilPow2(r.parityShards)
	n := ceilPow2(m + r.dataShards)

	const LEO_ERROR_BITFIELD_OPT = true

	// 填充错误位置。
	var errorBits errorBitfield8
	var errLocs [order8]ffe8
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

	if !gotInversion {
		// 没有反转...
		if LEO_ERROR_BITFIELD_OPT && useBits {
			errorBits.prepare()
		}

		// 评估错误定位多项式8
		fwht8(&errLocs, m+r.dataShards)

		for i := 0; i < order8; i++ {
			errLocs[i] = ffe8((uint(errLocs[i]) * uint(logWalsh8[i])) % modulus8)
		}

		fwht8(&errLocs, order8)

		if r.inversion != nil {
			c := leopardGF8cache{
				errorLocs: errLocs,
			}
			if useBits {
				// 堆分配
				var x errorBitfield8
				x = errorBits
				c.bits = &x
			}
			r.inversionMu.Lock()
			r.inversion[errorBits.cacheID()] = c
			r.inversionMu.Unlock()
		}
	}

	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
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
		work = make([][]byte, n)
		all := make([]byte, n*workSize8)
		for i := range work {
			work[i] = all[i*workSize8 : i*workSize8+workSize8]
		}
	}
	defer r.workPool.Put(work)

	// work <- recovery data

	// 分割大分片。
	// 在较低的分片数量上更有可能。
	sh := make([][]byte, len(shards))
	// 复制...
	copy(sh, shards)

	// 添加输出
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

	off := 0
	for off < shardSize {
		endSlice := off + workSize8
		if endSlice > shardSize {
			endSlice = shardSize
			sz := shardSize - off
			// 最后一次迭代
			for i := range work {
				work[i] = work[i][:sz]
			}
		}
		for i := range shards {
			if len(sh[i]) != 0 {
				sh[i] = shards[i][off:endSlice]
			}
		}
		for i := 0; i < r.parityShards; i++ {
			if len(sh[i+r.dataShards]) != 0 {
				mulgf8(work[i], sh[i+r.dataShards], errLocs[i], &r.o)
			} else {
				memclr(work[i])
			}
		}
		for i := r.parityShards; i < m; i++ {
			memclr(work[i])
		}

		// work <- 原始数据

		for i := 0; i < r.dataShards; i++ {
			if len(sh[i]) != 0 {
				mulgf8(work[m+i], sh[i], errLocs[m+i], &r.o)
			} else {
				memclr(work[m+i])
			}
		}
		for i := m + r.dataShards; i < n; i++ {
			memclr(work[i])
		}

		// work <- IFFT(work, n, 0)

		ifftDITDecoder8(
			m+r.dataShards,
			work,
			n,
			fftSkew8[:],
			&r.o,
		)

		// work <- FormalDerivative(work, n)

		for i := 1; i < n; i++ {
			width := ((i ^ (i - 1)) + 1) >> 1
			slicesXor(work[i-width:i], work[i:i+width], &r.o)
		}

		// work <- FFT(work, n, 0) truncated to m + dataShards

		outputCount := m + r.dataShards

		if LEO_ERROR_BITFIELD_OPT && useBits {
			errorBits.fftDIT8(work, outputCount, n, fftSkew8[:], &r.o)
		} else {
			fftDIT8(work, outputCount, n, fftSkew8[:], &r.o)
		}

		// 揭示擦除
		//
		//  Original = -ErrLocator * FFT( Derivative( IFFT( ErrLocator * ReceivedData ) ) )
		//  mul_mem(x, y, log_m, ) equals x[] = y[] * log_m
		//
		// mem layout: [Recovery Data (Power of Two = M)] [Original Data (K)] [Zero Padding out to N]
		end := r.dataShards
		if recoverAll {
			end = r.totalShards
		}
		// 恢复
		for i := 0; i < end; i++ {
			if len(sh[i]) != 0 {
				continue
			}

			if i >= r.dataShards {
				// 奇偶校验分片。
				mulgf8(shards[i][off:endSlice], work[i-r.dataShards], modulus8-errLocs[i-r.dataShards], &r.o)
			} else {
				// 数据分片。
				mulgf8(shards[i][off:endSlice], work[i+m], modulus8-errLocs[i+m], &r.o)
			}
		}
		off += workSize8
	}
	return nil
}

// 基本的没有花哨的版本用于解码器
func ifftDITDecoder8(mtrunc int, work [][]byte, m int, skewLUT []ffe8, o *options) {
	// 时间抽取:每次解卷积2层
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// 对于每个dist*4元素的集合:
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend-1]
			log_m02 := skewLUT[iend+dist-1]
			log_m23 := skewLUT[iend+dist*2-1]

			// 对于每个dist元素的集合:
			for i := r; i < iend; i++ {
				ifftDIT48(work[i:], dist, log_m01, log_m23, log_m02, o)
			}
		}
		dist = dist4
		dist4 <<= 2
	}

	// 如果只剩下一个层:
	if dist < m {
		// 假设dist = m / 2
		if dist*2 != m {
			panic("internal error")
		}

		log_m := skewLUT[dist-1]

		if log_m == modulus8 {
			slicesXor(work[dist:2*dist], work[:dist], o)
		} else {
			for i := 0; i < dist; i++ {
				ifftDIT28(
					work[i],
					work[i+dist],
					log_m,
					o,
				)
			}
		}
	}
}

// 在编码器和解码器中就地FFT
func fftDIT8(work [][]byte, mtrunc, m int, skewLUT []ffe8, o *options) {
	// 时间抽取:每次解卷积2层
	dist4 := m
	dist := m >> 2
	for dist != 0 {
		// 对于每个dist*4元素的集合:
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend-1]
			log_m02 := skewLUT[iend+dist-1]
			log_m23 := skewLUT[iend+dist*2-1]

			// 对于每个dist元素的集合:
			for i := r; i < iend; i++ {
				fftDIT48(
					work[i:],
					dist,
					log_m01,
					log_m23,
					log_m02,
					o,
				)
			}
		}
		dist4 = dist
		dist >>= 2
	}

	// 如果只剩下一个层:
	if dist4 == 2 {
		for r := 0; r < mtrunc; r += 2 {
			log_m := skewLUT[r+1-1]

			if log_m == modulus8 {
				sliceXor(work[r], work[r+1], o)
			} else {
				fftDIT28(work[r], work[r+1], log_m, o)
			}
		}
	}
}

// 4-way butterfly
func fftDIT4Ref8(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe8, o *options) {
	// 第一层:
	if log_m02 == modulus8 {
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		fftDIT28(work[0], work[dist*2], log_m02, o)
		fftDIT28(work[dist], work[dist*3], log_m02, o)
	}

	// 第二层:
	if log_m01 == modulus8 {
		sliceXor(work[0], work[dist], o)
	} else {
		fftDIT28(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus8 {
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		fftDIT28(work[dist*2], work[dist*3], log_m23, o)
	}
}

// 展开的IFFT用于编码器
func ifftDITEncoder8(data [][]byte, mtrunc int, work [][]byte, xorRes [][]byte, m int, skewLUT []ffe8, o *options) {
	// 我尝试将memcpy/memset滚动到FFT的第一层，
	// 发现它只提供4%的性能提升，这并不值得额外的复杂性。
	// 值得额外的复杂性。
	for i := 0; i < mtrunc; i++ {
		copy(work[i], data[i])
	}
	for i := mtrunc; i < m; i++ {
		memclr(work[i])
	}

	// 时间抽取:每次解卷积2层
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// 对于每个dist*4元素的集合:
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend]
			log_m02 := skewLUT[iend+dist]
			log_m23 := skewLUT[iend+dist*2]

			// 对于每个dist元素的集合:
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
		// 我尝试交替从左到右和从右到左的扫描来减少缓存未命中。
		// 当在FFT和IFFT中完成时，它提供了大约1%的性能提升，所以它似乎不值得额外的复杂性。
	}

	// 如果只剩下一个层:
	if dist < m {
		// 假设dist = m / 2
		if dist*2 != m {
			panic("internal error")
		}

		logm := skewLUT[dist]

		if logm == modulus8 {
			slicesXor(work[dist:dist*2], work[:dist], o)
		} else {
			for i := 0; i < dist; i++ {
				ifftDIT28(work[i], work[i+dist], logm, o)
			}
		}
	}

	// 我尝试展开这个，但它对16位有限域的性能提升不到5%，所以不值得复杂性。
	if xorRes != nil {
		slicesXor(xorRes[:m], work[:m], o)
	}
}

func ifftDIT4Ref8(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe8, o *options) {
	// 第一层:
	if log_m01 == modulus8 {
		sliceXor(work[0], work[dist], o)
	} else {
		ifftDIT28(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus8 {
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		ifftDIT28(work[dist*2], work[dist*3], log_m23, o)
	}

	// 第二层:
	if log_m02 == modulus8 {
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		ifftDIT28(work[0], work[dist*2], log_m02, o)
		ifftDIT28(work[dist], work[dist*3], log_m02, o)
	}
}

// 参考版本:x[] ^= y[] * log_m
func refMulAdd8(x, y []byte, log_m ffe8) {
	lut := &mul8LUTs[log_m]

	for len(x) >= 64 {
		// 断言大小以避免循环中的边界检查
		src := y[:64]
		dst := x[:len(src)] // 需要，但未检查...
		for i, y1 := range src {
			dst[i] ^= byte(lut.Value[y1])
		}
		x = x[64:]
		y = y[64:]
	}
}

// 参考版本:x[] = y[] * log_m
func refMul8(x, y []byte, log_m ffe8) {
	lut := &mul8LUTs[log_m]

	for off := 0; off < len(x); off += 64 {
		src := y[off : off+64]
		for i, y1 := range src {
			x[off+i] = byte(lut.Value[y1])
		}
	}
}

// 返回a * Log(b)
func mulLog8(a, log_b ffe8) ffe8 {
	/*
	   注意，这个操作不是在有限域中的正常乘法，因为右操作数已经是一个对数。
	   因为右操作数已经是一个对数，所以这个操作不是在有限域中的正常乘法。
	   这个操作将K表查找从Decode()方法移动到初始化步骤，这个步骤的性能不那么关键。
	   下面的LogWalsh[]表包含预计算的对数，所以也更容易以这种形式进行所有其他的乘法。
	*/
	if a == 0 {
		return 0
	}
	return expLUT8[addMod8(logLUT8[a], log_b)]
}

// z = x + y (mod kModulus)
func addMod8(a, b ffe8) ffe8 {
	sum := uint(a) + uint(b)

	// 部分约简步骤，允许返回kModulus
	return ffe8(sum + sum>>bitwidth8)
}

// z = x - y (mod kModulus)
func subMod8(a, b ffe8) ffe8 {
	dif := uint(a) - uint(b)

	// 部分约简步骤，允许返回kModulus
	return ffe8(dif + dif>>bitwidth8)
}

// 时间抽取(DIT)快速沃尔什-哈达玛变换
// 展开层对以在寄存器中执行跨层操作
// mtrunc: data中前端非零元素的数量
func fwht8(data *[order8]ffe8, mtrunc int) {
	// Decimation in time: Unroll 2 layers at a time
	dist := 1
	dist4 := 4
	for dist4 <= order8 {
		// 对于每个dist*4元素的集合:
		for r := 0; r < mtrunc; r += dist4 {
			// 对于每个dist元素的集合:
			// 使用16位索引避免[65536]ffe8的边界检查。
			dist := uint16(dist)
			off := uint16(r)
			for i := uint16(0); i < dist; i++ {
				// fwht48(data[i:], dist) inlined...
				// 读取值似乎比更新指针更快。
				// 转换为uint不是更快。
				t0 := data[off]
				t1 := data[off+dist]
				t2 := data[off+dist*2]
				t3 := data[off+dist*3]

				t0, t1 = fwht2alt8(t0, t1)
				t2, t3 = fwht2alt8(t2, t3)
				t0, t2 = fwht2alt8(t0, t2)
				t1, t3 = fwht2alt8(t1, t3)

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

func fwht48(data []ffe8, s int) {
	s2 := s << 1

	t0 := &data[0]
	t1 := &data[s]
	t2 := &data[s2]
	t3 := &data[s2+s]

	fwht28(t0, t1)
	fwht28(t2, t3)
	fwht28(t0, t2)
	fwht28(t1, t3)
}

// {a, b} = {a + b, a - b} (Mod Q)
func fwht28(a, b *ffe8) {
	sum := addMod8(*a, *b)
	dif := subMod8(*a, *b)
	*a = sum
	*b = dif
}

// fwht2alt8 是fwht28，但返回结果。
func fwht2alt8(a, b ffe8) (ffe8, ffe8) {
	return addMod8(a, b), subMod8(a, b)
}

var initOnce8 sync.Once

func initConstants8() {
	initOnce8.Do(func() {
		initLUTs8()
		initFFTSkew8()
		initMul8LUT()
	})
}

// 初始化logLUT8, expLUT8.
func initLUTs8() {
	cantorBasis := [bitwidth8]ffe8{
		1, 214, 152, 146, 86, 200, 88, 230,
	}

	expLUT8 = &[order8]ffe8{}
	logLUT8 = &[order8]ffe8{}

	// LFSR表生成:
	state := 1
	for i := ffe8(0); i < modulus8; i++ {
		expLUT8[state] = i
		state <<= 1
		if state >= order8 {
			state ^= polynomial8
		}
	}
	expLUT8[0] = modulus8

	// Cantor基础转换:

	logLUT8[0] = 0
	for i := 0; i < bitwidth8; i++ {
		basis := cantorBasis[i]
		width := 1 << i

		for j := 0; j < width; j++ {
			logLUT8[j+width] = logLUT8[j] ^ basis
		}
	}

	for i := 0; i < order8; i++ {
		logLUT8[i] = expLUT8[logLUT8[i]]
	}

	for i := 0; i < order8; i++ {
		expLUT8[logLUT8[i]] = ffe8(i)
	}

	expLUT8[modulus8] = expLUT8[0]
}

// 初始化fftSkew8.
func initFFTSkew8() {
	var temp [bitwidth8 - 1]ffe8

	// 生成FFT skew向量{1}:

	for i := 1; i < bitwidth8; i++ {
		temp[i-1] = ffe8(1 << i)
	}

	fftSkew8 = &[modulus8]ffe8{}
	logWalsh8 = &[order8]ffe8{}

	for m := 0; m < bitwidth8-1; m++ {
		step := 1 << (m + 1)

		fftSkew8[1<<m-1] = 0

		for i := m; i < bitwidth8-1; i++ {
			s := 1 << (i + 1)

			for j := 1<<m - 1; j < s; j += step {
				fftSkew8[j+s] = fftSkew8[j] ^ temp[i]
			}
		}

		temp[m] = modulus8 - logLUT8[mulLog8(temp[m], logLUT8[temp[m]^1])]

		for i := m + 1; i < bitwidth8-1; i++ {
			sum := addMod8(logLUT8[temp[i]^1], temp[m])
			temp[i] = mulLog8(temp[i], sum)
		}
	}

	for i := 0; i < modulus8; i++ {
		fftSkew8[i] = logLUT8[fftSkew8[i]]
	}

	// 预计算FWHT(Log[i]):

	for i := 0; i < order8; i++ {
		logWalsh8[i] = logLUT8[i]
	}
	logWalsh8[0] = 0

	fwht8(logWalsh8, order8)
}

func initMul8LUT() {
	mul8LUTs = &[order8]mul8LUT{}

	// 对于每个log_m乘数:
	for log_m := 0; log_m < order8; log_m++ {
		var tmp [64]ffe8
		for nibble, shift := 0, 0; nibble < 4; {
			nibble_lut := tmp[nibble*16:]

			for xnibble := 0; xnibble < 16; xnibble++ {
				prod := mulLog8(ffe8(xnibble<<shift), ffe8(log_m))
				nibble_lut[xnibble] = prod
			}
			nibble++
			shift += 4
		}
		lut := &mul8LUTs[log_m]
		for i := range lut.Value[:] {
			lut.Value[i] = tmp[i&15] ^ tmp[((i>>4)+16)]
		}
	}
	// 总是初始化汇编表。
	// 不像gf16那样是资源大户。
	if true {
		multiply256LUT8 = &[order8][16 * 2]byte{}

		for logM := range multiply256LUT8[:] {
			// 对于有限域宽度中的每个4位:
			shift := 0
			for i := 0; i < 2; i++ {
				// 构造16个条目的LUT用于PSHUFB
				prod := multiply256LUT8[logM][i*16 : i*16+16]
				for x := range prod[:] {
					prod[x] = byte(mulLog8(ffe8(x<<shift), ffe8(logM)))
				}
				shift += 4
			}
		}
	}
}

const kWords8 = order8 / 64

// errorBitfield包含渐进的错误以帮助指示哪些分片需要重建。
type errorBitfield8 struct {
	Words [7][kWords8]uint64
}

func (e *errorBitfield8) set(i int) {
	e.Words[0][(i/64)&3] |= uint64(1) << (i & 63)
}

func (e *errorBitfield8) cacheID() [inversion8Bytes]byte {
	var res [inversion8Bytes]byte
	binary.LittleEndian.PutUint64(res[0:8], e.Words[0][0])
	binary.LittleEndian.PutUint64(res[8:16], e.Words[0][1])
	binary.LittleEndian.PutUint64(res[16:24], e.Words[0][2])
	binary.LittleEndian.PutUint64(res[24:32], e.Words[0][3])
	return res
}

func (e *errorBitfield8) isNeeded(mipLevel, bit int) bool {
	if mipLevel >= 8 || mipLevel <= 0 {
		return true
	}
	return 0 != (e.Words[mipLevel-1][bit/64] & (uint64(1) << (bit & 63)))
}

func (e *errorBitfield8) prepare() {
	// 第一个mip级别是FFT的最后一层:数据对
	for i := 0; i < kWords8; i++ {
		w_i := e.Words[0][i]
		hi2lo0 := w_i | ((w_i & kHiMasks[0]) >> 1)
		lo2hi0 := (w_i & (kHiMasks[0] >> 1)) << 1
		w_i = hi2lo0 | lo2hi0
		e.Words[0][i] = w_i

		bits := 2
		for j := 1; j < 5; j++ {
			hi2lo_j := w_i | ((w_i & kHiMasks[j]) >> bits)
			lo2hi_j := (w_i & (kHiMasks[j] >> bits)) << bits
			w_i = hi2lo_j | lo2hi_j
			e.Words[j][i] = w_i
			bits <<= 1
		}
	}

	for i := 0; i < kWords8; i++ {
		w := e.Words[4][i]
		w |= w >> 32
		w |= w << 32
		e.Words[5][i] = w
	}

	for i := 0; i < kWords8; i += 2 {
		t := e.Words[5][i] | e.Words[5][i+1]
		e.Words[6][i] = t
		e.Words[6][i+1] = t
	}
}

func (e *errorBitfield8) fftDIT8(work [][]byte, mtrunc, m int, skewLUT []ffe8, o *options) {
	// 时间抽取:展开2层一次
	mipLevel := bits.Len32(uint32(m)) - 1

	dist4 := m
	dist := m >> 2
	for dist != 0 {
		// 对于每个dist*4元素的集合:
		for r := 0; r < mtrunc; r += dist4 {
			if !e.isNeeded(mipLevel, r) {
				continue
			}
			iEnd := r + dist
			logM01 := skewLUT[iEnd-1]
			logM02 := skewLUT[iEnd+dist-1]
			logM23 := skewLUT[iEnd+dist*2-1]

			// 对于每个dist元素的集合:
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

	// 如果剩下一个层:
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
