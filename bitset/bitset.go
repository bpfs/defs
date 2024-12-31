/*
Package bitset 实现了位集合，它是一个在非负整数和布尔值之间的映射。
相比于 map[uint]bool，它的效率更高。

它提供了设置、清除、翻转和测试单个整数的方法。

同时也提供了集合的交集、并集、差集、补集和对称操作，以及测试是否有任何位被设置、
所有位被设置或没有位被设置的方法，并可以查询位集合的当前长度和设置位的数量。

位集合会扩展到最大设置位的大小；内存分配大约是最大位数，其中最大位是最大的设置位。
位集合永远不会缩小。在创建时，可以给出将要使用的位数的提示。

许多方法，包括 Set、Clear 和 Flip，都返回一个 BitSet 指针，这允许链式调用。

使用示例:

	import "bitset"
	var b BitSet
	b.Set(10).Set(11)
	if b.Test(1000) {
		b.Clear(1000)
	}
	if B.Intersection(bitset.New(100).Set(10)).Count() > 1 {
		fmt.Println("Intersection works.")
	}

作为 BitSets 的替代方案，应该查看 'big' 包，它提供了位集合的(较少集合理论)视图。
*/
package bitset

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// wordSize 定义了位集合中一个字的大小(以位为单位)
const wordSize = uint(64)

// wordBytes 定义了位集合中一个字的大小(以字节为单位)
const wordBytes = wordSize / 8

// log2WordSize 是 wordSize 的以2为底的对数
const log2WordSize = uint(6)

// allBits 是一个所有位都设置为1的掩码
const allBits uint64 = 0xffffffffffffffff

// binaryOrder 定义了二进制编码的字节序，默认为大端序
var binaryOrder binary.ByteOrder = binary.BigEndian

// base64Encoding 定义了 JSON 编码使用的 base64 编码方式，默认为 URLEncoding
var base64Encoding = base64.URLEncoding

// Base64StdEncoding 设置 Marshal/Unmarshal BitSet 使用标准的 base64 编码
// (默认使用 base64.URLEncoding)
func Base64StdEncoding() { base64Encoding = base64.StdEncoding }

// LittleEndian 设置 Marshal/Unmarshal 二进制使用小端序
// (默认使用 binary.BigEndian)
func LittleEndian() { binaryOrder = binary.LittleEndian }

// BigEndian 设置 Marshal/Unmarshal 二进制使用大端序
// (默认使用 binary.BigEndian)
func BigEndian() { binaryOrder = binary.BigEndian }

// BinaryOrder 返回当前的二进制字节序
// 参见 LittleEndian() 和 BigEndian() 来更改字节序
func BinaryOrder() binary.ByteOrder { return binaryOrder }

// BitSet 是一个位集合。BitSet 的零值是一个长度为 0 的空集合。
type BitSet struct {
	length uint     // 位集合的长度(位数)
	set    []uint64 // 存储位的切片，每个元素存储 64 位
}

// Error 用于区分此包中生成的错误(panic)
type Error string

// safeSet 确保 b.set 不为 nil 并返回该字段值
func (b *BitSet) safeSet() []uint64 {
	if b.set == nil {
		b.set = make([]uint64, wordsNeeded(0))
	}
	return b.set
}

// SetBitsetFrom 使用整数数组填充位集合，而不创建新的 BitSet 实例
// 参数:
//   - buf: []uint64 用于填充位集合的整数数组
func (b *BitSet) SetBitsetFrom(buf []uint64) {
	b.length = uint(len(buf)) * 64
	b.set = buf
}

// From 是一个构造函数，用于从字数组创建 BitSet
// 参数:
//   - buf: []uint64 用于创建位集合的字数组
//
// 返回值:
//   - *BitSet: 新创建的位集合
func From(buf []uint64) *BitSet {
	return FromWithLength(uint(len(buf))*64, buf)
}

// FromWithLength 从字数组和长度构造位集合
// 参数:
//   - len: uint 位集合的长度
//   - set: []uint64 用于创建位集合的字数组
//
// 返回值:
//   - *BitSet: 新创建的位集合
func FromWithLength(len uint, set []uint64) *BitSet {
	return &BitSet{len, set}
}

// Bytes 返回位集合作为 64 位字的数组，提供对内部表示的直接访问
// 返回的不是副本，因此对返回的切片的更改会影响位集合
// 这是为高级用户设计的
// 返回值:
//   - []uint64: 位集合的内部表示
func (b *BitSet) Bytes() []uint64 {
	return b.set
}

// wordsNeeded 计算存储 i 位需要的字数
// 参数:
//   - i: uint 位数
//
// 返回值:
//   - int: 需要的字数
func wordsNeeded(i uint) int {
	if i > (Cap() - wordSize + 1) {
		return int(Cap() >> log2WordSize)
	}
	return int((i + (wordSize - 1)) >> log2WordSize)
}

// wordsNeededUnbound 计算存储 i 位需要的字数，可能超过容量
// 当你知道不会超过容量时(例如，你有一个现有的 BitSet)，这个函数很有用
// 参数:
//   - i: uint 位数
//
// 返回值:
//   - int: 需要的字数
func wordsNeededUnbound(i uint) int {
	return int((i + (wordSize - 1)) >> log2WordSize)
}

// wordsIndex 计算在 uint64 中的字索引
// 参数:
//   - i: uint 位索引
//
// 返回值:
//   - uint: 字中的位索引
func wordsIndex(i uint) uint {
	return i & (wordSize - 1)
}

// New 创建一个新的 BitSet，提示将需要 length 位
// 参数:
//   - length: uint 预期的位数
//
// 返回值:
//   - *BitSet: 新创建的位集合
func New(length uint) (bset *BitSet) {
	defer func() {
		if r := recover(); r != nil {
			bset = &BitSet{
				0,
				make([]uint64, 0),
			}
		}
	}()

	bset = &BitSet{
		length,
		make([]uint64, wordsNeeded(length)),
	}

	return bset
}

// Cap 返回可以存储在 BitSet 中的位的理论总容量
// 在 32 位系统上是 4294967295，在 64 位系统上是 18446744073709551615
// 注意这进一步受限于 Go 中的最大分配大小和可用内存，就像任何 Go 数据结构一样
// 返回值:
//   - uint: 最大容量
func Cap() uint {
	return ^uint(0)
}

// Len 返回 BitSet 中的位数
// 注意它与 Count 函数不同
// 返回值:
//   - uint: 位集合的长度
func (b *BitSet) Len() uint {
	return b.length
}

// extendSet 添加额外的字以包含新位(如果需要)
// 参数:
//   - i: uint 需要扩展到的位索引
func (b *BitSet) extendSet(i uint) {
	if i >= Cap() {
		panic("You are exceeding the capacity")
	}
	nsize := wordsNeeded(i + 1)
	if b.set == nil {
		b.set = make([]uint64, nsize)
	} else if cap(b.set) >= nsize {
		b.set = b.set[:nsize] // 快速调整大小
	} else if len(b.set) < nsize {
		newset := make([]uint64, nsize, 2*nsize) // 容量增加2倍
		copy(newset, b.set)
		b.set = newset
	}
	b.length = i + 1
}

// Test 测试位 i 是否被设置
// 参数:
//   - i: uint 要测试的位索引
//
// 返回值:
//   - bool: 如果位被设置则返回 true，否则返回 false
func (b *BitSet) Test(i uint) bool {
	if i >= b.length {
		return false
	}
	return b.set[i>>log2WordSize]&(1<<wordsIndex(i)) != 0
}

// Set 将位 i 设置为 1，位集合的容量会相应自动增加
// 警告：对 'i' 使用非常大的值可能导致内存不足和 panic：
// 调用者负责根据其内存容量提供合理的参数
// 内存使用量至少略高于 i/8 字节
// 参数:
//   - i: uint 要设置的位索引
//
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) Set(i uint) *BitSet {
	if i >= b.length { // 如果我们需要更多位，就创建它们
		b.extendSet(i)
	}
	b.set[i>>log2WordSize] |= 1 << wordsIndex(i)
	return b
}

// Clear 将位 i 设置为 0。这永远不会导致内存分配。它总是安全的
// 参数:
//   - i: uint 要清除的位索引
//
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) Clear(i uint) *BitSet {
	if i >= b.length {
		return b
	}
	b.set[i>>log2WordSize] &^= 1 << wordsIndex(i)
	return b
}

// SetTo 将位 i 设置为指定的值
// 警告：对 'i' 使用非常大的值可能导致内存不足和 panic：
// 调用者负责根据其内存容量提供合理的参数
// 参数:
//   - i: uint 要设置的位索引
//   - value: bool 要设置的值
//
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) SetTo(i uint, value bool) *BitSet {
	if value {
		return b.Set(i)
	}
	return b.Clear(i)
}

// Flip 翻转位 i
// 警告：对 'i' 使用非常大的值可能导致内存不足和 panic：
// 调用者负责根据其内存容量提供合理的参数
// 参数:
//   - i: uint 要翻转的位索引
//
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) Flip(i uint) *BitSet {
	if i >= b.length {
		return b.Set(i)
	}
	b.set[i>>log2WordSize] ^= 1 << wordsIndex(i)
	return b
}

// FlipRange 翻转区间 [start, end) 中的位
// 警告：对 'end' 使用非常大的值可能导致内存不足和 panic：
// 调用者负责根据其内存容量提供合理的参数
// 参数:
//   - start: uint 起始位索引(包含)
//   - end: uint 结束位索引(不包含)
//
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) FlipRange(start, end uint) *BitSet {
	if start >= end {
		return b
	}
	if end-1 >= b.length { // 如果我们需要更多位，就创建它们
		b.extendSet(end - 1)
	}
	var startWord uint = start >> log2WordSize
	var endWord uint = end >> log2WordSize
	b.set[startWord] ^= ^(^uint64(0) << wordsIndex(start))
	if endWord > 0 {
		// 边界检查消除
		data := b.set
		_ = data[endWord-1]
		for i := startWord; i < endWord; i++ {
			data[i] = ^data[i]
		}
	}
	if end&(wordSize-1) != 0 {
		b.set[endWord] ^= ^uint64(0) >> wordsIndex(-end)
	}
	return b
}

// Shrink 缩小位集合，使得提供的值成为最后一个可能设置的值
// 它清除所有大于提供的索引的位，并减少集合的大小和长度
// 注意：参数值不是新的位长度：它是函数调用后可以存储在位集合中的最大值
// 新的位长度是参数值 + 1。因此不可能使用此函数将长度设置为 0，
// 此函数调用后的最小长度值为 1
//
// 分配一个新的切片来存储新的位，所以在 GC 运行之前你可能会看到内存使用增加
// 通常这不应该是问题，但如果你有一个极大的位集合，重要的是要理解旧的位集合
// 将保留在内存中，直到 GC 释放它
// 如果内存受限，此函数可能会导致 panic
// 参数:
//   - lastbitindex: uint 最后一个可能设置的位的索引
//
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) Shrink(lastbitindex uint) *BitSet {
	// 计算新的长度
	length := lastbitindex + 1
	// 计算需要的字数
	idx := wordsNeeded(length)
	// 如果需要的字数大于当前集合，则不做任何更改
	if idx > len(b.set) {
		return b
	}
	// 创建新的切片存储缩小后的数据
	shrunk := make([]uint64, idx)
	// 复制数据到新切片
	copy(shrunk, b.set[:idx])
	// 更新集合的切片和长度
	b.set = shrunk
	b.length = length
	// 处理最后一个字的未使用位
	lastWordUsedBits := length % 64
	if lastWordUsedBits != 0 {
		b.set[idx-1] &= allBits >> uint64(64-wordsIndex(lastWordUsedBits))
	}
	return b
}

// Compact 缩小位集合以保留所有设置的位，同时最小化内存使用
// 此函数会调用 Shrink
// 分配一个新的切片来存储新的位，所以在 GC 运行之前你可能会看到内存使用增加
// 通常这不应该是问题，但如果你有一个极大的位集合，重要的是要理解旧的位集合
// 将保留在内存中，直到 GC 释放它
// 如果内存受限，此函数可能会导致 panic
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) Compact() *BitSet {
	// 从后向前查找最后一个非零字
	idx := len(b.set) - 1
	for ; idx >= 0 && b.set[idx] == 0; idx-- {
	}
	// 计算新的长度
	newlength := uint((idx + 1) << log2WordSize)
	if newlength >= b.length {
		return b // 无需更改
	}
	if newlength > 0 {
		return b.Shrink(newlength - 1)
	}
	// 保留一个字
	return b.Shrink(63)
}

// InsertAt 在指定索引处插入一个位
// 从给定的索引位置开始，将集合中的所有位向左移动 1 位，
// 并将索引位置设置为 0
// 根据位集合的大小和插入位置，此方法可能会非常慢，
// 在某些情况下可能会导致整个位集合被重新复制
// 参数:
//   - idx: uint 要插入位的索引
//
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) InsertAt(idx uint) *BitSet {
	// 计算要插入的元素索引
	insertAtElement := idx >> log2WordSize

	// 如果集合长度是 wordSize 的倍数，需要先分配更多空间
	if b.isLenExactMultiple() {
		b.set = append(b.set, uint64(0))
	}

	// 从后向前移动位
	var i uint
	for i = uint(len(b.set) - 1); i > insertAtElement; i-- {
		// 当前元素左移一位
		b.set[i] <<= 1
		// 将前一个元素的最高位设置为当前元素的最低位
		b.set[i] |= (b.set[i-1] & 0x8000000000000000) >> 63
	}

	// 生成掩码以提取需要左移的数据
	dataMask := uint64(1)<<uint64(wordsIndex(idx)) - 1
	// 提取需要移动的数据
	data := b.set[i] & (^dataMask)
	// 将插入位置的数据掩码位置为 0
	b.set[i] &= dataMask
	// 左移数据并插入到切片元素中
	b.set[i] |= data << 1
	// 增加位集合的长度
	b.length++

	return b
}

// String 创建位集合的字符串表示
// 仅用于人类可读输出，不用于序列化
// 返回值:
//   - string: 位集合的字符串表示
func (b *BitSet) String() string {
	// 创建一个字节缓冲区
	var buffer bytes.Buffer
	// 写入开始标记
	start := []byte("{")
	buffer.Write(start)
	// 计数器用于限制输出大小
	counter := 0
	// 遍历所有设置的位
	i, e := b.NextSet(0)
	for e {
		counter = counter + 1
		// 避免耗尽内存
		if counter > 0x40000 {
			buffer.WriteString("...")
			break
		}
		// 写入位的索引
		buffer.WriteString(strconv.FormatInt(int64(i), 10))
		i, e = b.NextSet(i + 1)
		if e {
			buffer.WriteString(",")
		}
	}
	buffer.WriteString("}")
	return buffer.String()
}

// DeleteAt 从位集合中删除指定索引位置的位
// 被删除位左侧的所有位向右移动 1 位
// 此操作的运行时间可能相对较慢，为 O(length)
// 参数:
//   - i: uint 要删除的位的索引
//
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) DeleteAt(i uint) *BitSet {
	// 计算要删除位所在的元素索引
	deleteAtElement := i >> log2WordSize

	// 生成需要右移数据的掩码
	dataMask := ^((uint64(1) << wordsIndex(i)) - 1)
	// 提取需要右移的数据
	data := b.set[deleteAtElement] & dataMask
	// 将掩码区域置零，保留其余部分
	b.set[deleteAtElement] &= ^dataMask
	// 右移提取的数据并设置到之前掩码的区域
	b.set[deleteAtElement] |= (data >> 1) & dataMask

	// 遍历所有后续元素
	for i := int(deleteAtElement) + 1; i < len(b.set); i++ {
		// 将当前元素的最低位复制到前一个元素的最高位
		b.set[i-1] |= (b.set[i] & 1) << 63
		// 将当前元素右移一位
		b.set[i] >>= 1
	}

	// 减少位集合的长度
	b.length = b.length - 1

	return b
}

// NextSet 返回从指定索引开始的下一个设置的位
// 包括可能的当前索引，同时返回一个错误代码
// (true = 有效, false = 未找到设置的位)
// 参数:
//   - i: uint 开始搜索的索引
//
// 返回值:
//   - uint: 下一个设置的位的索引
//   - bool: 是否找到设置的位
func (b *BitSet) NextSet(i uint) (uint, bool) {
	// 计算起始字的索引
	x := int(i >> log2WordSize)
	if x >= len(b.set) {
		return 0, false
	}
	// 获取当前字并右移到起始位置
	w := b.set[x]
	w = w >> wordsIndex(i)
	if w != 0 {
		return i + trailingZeroes64(w), true
	}
	// 继续查找后续的字
	x++
	if x < 0 {
		return 0, false
	}
	for x < len(b.set) {
		if b.set[x] != 0 {
			return uint(x)*wordSize + trailingZeroes64(b.set[x]), true
		}
		x++
	}
	return 0, false
}

// NextSetMany returns many next bit sets from the specified index,
// including possibly the current index and up to cap(buffer).
// If the returned slice has len zero, then no more set bits were found
//
//	buffer := make([]uint, 256) // this should be reused
//	j := uint(0)
//	j, buffer = bitmap.NextSetMany(j, buffer)
//	for ; len(buffer) > 0; j, buffer = bitmap.NextSetMany(j,buffer) {
//	 for k := range buffer {
//	  do something with buffer[k]
//	 }
//	 j += 1
//	}
//
// It is possible to retrieve all set bits as follow:
//
//	indices := make([]uint, bitmap.Count())
//	bitmap.NextSetMany(0, indices)
//
// However if bitmap.Count() is large, it might be preferable to
// use several calls to NextSetMany, for performance reasons.
func (b *BitSet) NextSetMany(i uint, buffer []uint) (uint, []uint) {
	myanswer := buffer
	capacity := cap(buffer)
	x := int(i >> log2WordSize)
	if x >= len(b.set) || capacity == 0 {
		return 0, myanswer[:0]
	}
	skip := wordsIndex(i)
	word := b.set[x] >> skip
	myanswer = myanswer[:capacity]
	size := int(0)
	for word != 0 {
		r := trailingZeroes64(word)
		t := word & ((^word) + 1)
		myanswer[size] = r + i
		size++
		if size == capacity {
			goto End
		}
		word = word ^ t
	}
	x++
	for idx, word := range b.set[x:] {
		for word != 0 {
			r := trailingZeroes64(word)
			t := word & ((^word) + 1)
			myanswer[size] = r + (uint(x+idx) << 6)
			size++
			if size == capacity {
				goto End
			}
			word = word ^ t
		}
	}
End:
	if size > 0 {
		return myanswer[size-1], myanswer[:size]
	}
	return 0, myanswer[:0]
}

// NextClear 从指定索引开始查找下一个未设置的位，包括当前索引
// 参数:
//   - i: uint 开始查找的位索引
//
// 返回值:
//   - uint: 找到的第一个未设置位的索引
//   - bool: 是否找到未设置位(true=找到, false=未找到,即所有位都被设置)
func (b *BitSet) NextClear(i uint) (uint, bool) {
	// 计算字索引
	x := int(i >> log2WordSize)
	if x >= len(b.set) {
		return 0, false
	}
	// 获取当前字
	w := b.set[x]
	// 右移到指定位置
	w = w >> wordsIndex(i)
	// 计算掩码
	wA := allBits >> wordsIndex(i)
	// 计算下一个未设置位的索引
	index := i + trailingZeroes64(^w)
	// 如果当前字中找到未设置位且索引在有效范围内
	if w != wA && index < b.length {
		return index, true
	}
	// 继续查找下一个字
	x++
	// 边界检查消除
	if x < 0 {
		return 0, false
	}
	// 遍历剩余的字
	for x < len(b.set) {
		if b.set[x] != allBits {
			// 找到包含未设置位的字,计算具体位置
			index = uint(x)*wordSize + trailingZeroes64(^b.set[x])
			if index < b.length {
				return index, true
			}
		}
		x++
	}
	return 0, false
}

// ClearAll 清除整个位集合中的所有位
// 不会释放内存
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) ClearAll() *BitSet {
	if b != nil && b.set != nil {
		// 遍历所有字并设置为0
		for i := range b.set {
			b.set[i] = 0
		}
	}
	return b
}

// SetAll 设置整个位集合中的所有位
// 返回值:
//   - *BitSet: 位集合本身，用于链式调用
func (b *BitSet) SetAll() *BitSet {
	if b != nil && b.set != nil {
		// 遍历所有字并设置为全1
		for i := range b.set {
			b.set[i] = allBits
		}
		// 清理最后一个字中超出长度的位
		b.cleanLastWord()
	}
	return b
}

// wordCount 返回位集合使用的字数
// 返回值:
//   - int: 使用的字数
func (b *BitSet) wordCount() int {
	return wordsNeededUnbound(b.length)
}

// Clone 克隆当前位集合
// 返回值:
//   - *BitSet: 克隆的新位集合
func (b *BitSet) Clone() *BitSet {
	// 创建相同长度的新位集合
	c := New(b.length)
	if b.set != nil { // 克隆不应修改当前对象
		// 复制数据
		copy(c.set, b.set)
	}
	return c
}

// Copy 将当前位集合复制到目标位集合
// 使用Go数组复制语义:复制的位数是当前位集合(Len())和目标位集合中位数的较小值
// 参数:
//   - c: *BitSet 目标位集合
//
// 返回值:
//   - count: uint 复制到目标位集合的位数
func (b *BitSet) Copy(c *BitSet) (count uint) {
	if c == nil {
		return
	}
	if b.set != nil { // 复制不应修改当前对象
		// 复制数据
		copy(c.set, b.set)
	}
	// 计算复制的位数
	count = c.length
	if b.length < c.length {
		count = b.length
	}
	// 清理最后一个字,确保超出长度的位为0
	c.cleanLastWord()
	return
}

// CopyFull 将当前位集合完整复制到目标位集合
// 操作后目标位集合与源位集合完全相同,必要时会分配内存
// 参数:
//   - c: *BitSet 目标位集合
func (b *BitSet) CopyFull(c *BitSet) {
	if c == nil {
		return
	}
	// 设置长度
	c.length = b.length
	if len(b.set) == 0 {
		// 如果源为空,清空目标
		if c.set != nil {
			c.set = c.set[:0]
		}
	} else {
		// 确保目标有足够容量
		if cap(c.set) < len(b.set) {
			c.set = make([]uint64, len(b.set))
		} else {
			c.set = c.set[:len(b.set)]
		}
		// 复制数据
		copy(c.set, b.set)
	}
}

// Count 返回设置位的数量
// 也称为"popcount"或"population count"
// 返回值:
//   - uint: 设置位的数量
func (b *BitSet) Count() uint {
	if b != nil && b.set != nil {
		return uint(popcntSlice(b.set))
	}
	return 0
}

// Equal 测试两个位集合是否相等
// 如果长度不同则返回false,否则只有当所有相同的位被设置时才返回true
// 参数:
//   - c: *BitSet 要比较的位集合
//
// 返回值:
//   - bool: 两个位集合是否相等
func (b *BitSet) Equal(c *BitSet) bool {
	if c == nil || b == nil {
		return c == b
	}
	if b.length != c.length {
		return false
	}
	if b.length == 0 { // 如果两者长度都为0,则可能有nil的set
		return true
	}
	// 获取字数
	wn := b.wordCount()
	// 边界检查消除
	if wn <= 0 {
		return true
	}
	_ = b.set[wn-1]
	_ = c.set[wn-1]
	// 比较每个字
	for p := 0; p < wn; p++ {
		if c.set[p] != b.set[p] {
			return false
		}
	}
	return true
}

// panicIfNull 如果位集合为nil则panic
// 参数:
//   - b: *BitSet 要检查的位集合
func panicIfNull(b *BitSet) {
	if b == nil {
		panic(Error("BitSet must not be null"))
	}
}

// Difference 计算基集合与其他集合的差集
// 这是BitSet的&^(与非)运算等价操作
// 参数:
//   - compare: *BitSet 要计算差集的位集合
//
// 返回值:
//   - *BitSet: 差集结果
func (b *BitSet) Difference(compare *BitSet) (result *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	// 克隆b(以防b比compare大)
	result = b.Clone()
	// 计算要处理的字数
	l := compare.wordCount()
	if l > b.wordCount() {
		l = b.wordCount()
	}
	// 计算每个字的差集
	for i := 0; i < l; i++ {
		result.set[i] = b.set[i] &^ compare.set[i]
	}
	return
}

// DifferenceCardinality 计算差集的基数
// 参数:
//   - compare: *BitSet 要计算差集的位集合
//
// 返回值:
//   - uint: 差集中的位数
func (b *BitSet) DifferenceCardinality(compare *BitSet) uint {
	panicIfNull(b)
	panicIfNull(compare)
	// 计算要处理的字数
	l := compare.wordCount()
	if l > b.wordCount() {
		l = b.wordCount()
	}
	cnt := uint64(0)
	// 计算重叠部分的差集位数
	cnt += popcntMaskSlice(b.set[:l], compare.set[:l])
	// 加上b中剩余部分的位数
	cnt += popcntSlice(b.set[l:])
	return uint(cnt)
}

// InPlaceDifference 就地计算基集合与其他集合的差集
// 这是BitSet的&^(与非)运算等价操作
// 参数:
//   - compare: *BitSet 要计算差集的位集合
func (b *BitSet) InPlaceDifference(compare *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	// 计算要处理的字数
	l := compare.wordCount()
	if l > b.wordCount() {
		l = b.wordCount()
	}
	if l <= 0 {
		return
	}
	// 边界检查消除
	data, cmpData := b.set, compare.set
	_ = data[l-1]
	_ = cmpData[l-1]
	// 计算每个字的差集
	for i := 0; i < l; i++ {
		data[i] &^= cmpData[i]
	}
}

// sortByLength 便利函数:按长度递增顺序返回两个位集合
// 注意:两个参数都不能为nil
// 参数:
//   - a: *BitSet 第一个位集合
//   - b: *BitSet 第二个位集合
//
// 返回值:
//   - ap: *BitSet 较短的位集合
//   - bp: *BitSet 较长的位集合
func sortByLength(a *BitSet, b *BitSet) (ap *BitSet, bp *BitSet) {
	if a.length <= b.length {
		ap, bp = a, b
	} else {
		ap, bp = b, a
	}
	return
}

// Intersection 计算基集合与其他集合的交集
// 这是BitSet的&(与)运算等价操作
// 参数:
//   - compare: *BitSet 要计算交集的位集合
//
// 返回值:
//   - *BitSet: 交集结果
func (b *BitSet) Intersection(compare *BitSet) (result *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	// 按长度排序
	b, compare = sortByLength(b, compare)
	// 创建结果集合
	result = New(b.length)
	// 计算每个字的交集
	for i, word := range b.set {
		result.set[i] = word & compare.set[i]
	}
	return
}

// IntersectionCardinality 计算两个位集合交集的基数(设置为1的位的数量)
// 参数:
//   - compare: *BitSet 要计算交集的位集合
//
// 返回值:
//   - uint: 交集中设置为1的位的数量
func (b *BitSet) IntersectionCardinality(compare *BitSet) uint {
	// 检查参数是否为空
	panicIfNull(b)
	panicIfNull(compare)
	// 按长度排序,确保b是较短的集合
	b, compare = sortByLength(b, compare)
	// 计算两个集合按位与后1的个数
	cnt := popcntAndSlice(b.set, compare.set)
	return uint(cnt)
}

// InPlaceIntersection 就地计算基集合与比较集合的交集
// 这是BitSet的&(与)运算等价操作,会修改基集合
// 参数:
//   - compare: *BitSet 要计算交集的位集合
func (b *BitSet) InPlaceIntersection(compare *BitSet) {
	// 检查参数是否为空
	panicIfNull(b)
	panicIfNull(compare)
	// 获取两个集合中较小的字数
	l := compare.wordCount()
	if l > b.wordCount() {
		l = b.wordCount()
	}
	if l > 0 {
		// 边界检查消除
		data, cmpData := b.set, compare.set
		_ = data[l-1]
		_ = cmpData[l-1]

		// 计算每个字的交集
		for i := 0; i < l; i++ {
			data[i] &= cmpData[i]
		}
	}
	if l >= 0 {
		// 将超出比较集合长度的位清零
		for i := l; i < len(b.set); i++ {
			b.set[i] = 0
		}
	}
	// 如果需要,扩展基集合长度
	if compare.length > 0 {
		if compare.length-1 >= b.length {
			b.extendSet(compare.length - 1)
		}
	}
}

// Union 计算基集合与其他集合的并集
// 这是BitSet的|(或)运算等价操作
// 参数:
//   - compare: *BitSet 要计算并集的位集合
//
// 返回值:
//   - result: *BitSet 并集结果
func (b *BitSet) Union(compare *BitSet) (result *BitSet) {
	// 检查参数是否为空
	panicIfNull(b)
	panicIfNull(compare)
	// 按长度排序,确保b是较短的集合
	b, compare = sortByLength(b, compare)
	// 克隆较长的集合作为结果
	result = compare.Clone()
	// 计算每个字的并集
	for i, word := range b.set {
		result.set[i] = word | compare.set[i]
	}
	return
}

// UnionCardinality 计算两个位集合并集的基数(设置为1的位的数量)
// 参数:
//   - compare: *BitSet 要计算并集的位集合
//
// 返回值:
//   - uint: 并集中设置为1的位的数量
func (b *BitSet) UnionCardinality(compare *BitSet) uint {
	// 检查参数是否为空
	panicIfNull(b)
	panicIfNull(compare)
	// 按长度排序,确保b是较短的集合
	b, compare = sortByLength(b, compare)
	// 计算重叠部分的并集中1的个数
	cnt := popcntOrSlice(b.set, compare.set)
	// 加上较长集合多出部分的1的个数
	if len(compare.set) > len(b.set) {
		cnt += popcntSlice(compare.set[len(b.set):])
	}
	return uint(cnt)
}

// InPlaceUnion 就地计算基集合与比较集合的并集
// 这是BitSet的|(或)运算等价操作,会修改基集合
// 参数:
//   - compare: *BitSet 要计算并集的位集合
func (b *BitSet) InPlaceUnion(compare *BitSet) {
	// 检查参数是否为空
	panicIfNull(b)
	panicIfNull(compare)
	// 获取两个集合中较小的字数
	l := compare.wordCount()
	if l > b.wordCount() {
		l = b.wordCount()
	}
	// 如果需要,扩展基集合长度
	if compare.length > 0 && compare.length-1 >= b.length {
		b.extendSet(compare.length - 1)
	}
	if l > 0 {
		// 边界检查消除
		data, cmpData := b.set, compare.set
		_ = data[l-1]
		_ = cmpData[l-1]

		// 计算每个字的并集
		for i := 0; i < l; i++ {
			data[i] |= cmpData[i]
		}
	}
	// 复制比较集合多出的部分
	if len(compare.set) > l {
		for i := l; i < len(compare.set); i++ {
			b.set[i] = compare.set[i]
		}
	}
}

// SymmetricDifference 计算基集合与其他集合的对称差
// 这是BitSet的^(异或)运算等价操作
// 参数:
//   - compare: *BitSet 要计算对称差的位集合
//
// 返回值:
//   - result: *BitSet 对称差结果
func (b *BitSet) SymmetricDifference(compare *BitSet) (result *BitSet) {
	// 检查参数是否为空
	panicIfNull(b)
	panicIfNull(compare)
	// 按长度排序,确保b是较短的集合
	b, compare = sortByLength(b, compare)
	// 克隆较长的集合作为结果
	result = compare.Clone()
	// 计算每个字的对称差
	for i, word := range b.set {
		result.set[i] = word ^ compare.set[i]
	}
	return
}

// SymmetricDifferenceCardinality 计算两个位集合对称差的基数(设置为1的位的数量)
// 参数:
//   - compare: *BitSet 要计算对称差的位集合
//
// 返回值:
//   - uint: 对称差中设置为1的位的数量
func (b *BitSet) SymmetricDifferenceCardinality(compare *BitSet) uint {
	// 检查参数是否为空
	panicIfNull(b)
	panicIfNull(compare)
	// 按长度排序,确保b是较短的集合
	b, compare = sortByLength(b, compare)
	// 计算重叠部分的对称差中1的个数
	cnt := popcntXorSlice(b.set, compare.set)
	// 加上较长集合多出部分的1的个数
	if len(compare.set) > len(b.set) {
		cnt += popcntSlice(compare.set[len(b.set):])
	}
	return uint(cnt)
}

// InPlaceSymmetricDifference 就地计算基集合与比较集合的对称差
// 这是BitSet的^(异或)运算等价操作,会修改基集合
// 参数:
//   - compare: *BitSet 要计算对称差的位集合
func (b *BitSet) InPlaceSymmetricDifference(compare *BitSet) {
	// 检查参数是否为空
	panicIfNull(b)
	panicIfNull(compare)
	// 获取两个集合中较小的字数
	l := compare.wordCount()
	if l > b.wordCount() {
		l = b.wordCount()
	}
	// 如果需要,扩展基集合长度
	if compare.length > 0 && compare.length-1 >= b.length {
		b.extendSet(compare.length - 1)
	}
	if l > 0 {
		// 边界检查消除
		data, cmpData := b.set, compare.set
		_ = data[l-1]
		_ = cmpData[l-1]
		// 计算每个字的对称差
		for i := 0; i < l; i++ {
			data[i] ^= cmpData[i]
		}
	}
	// 复制比较集合多出的部分
	if len(compare.set) > l {
		for i := l; i < len(compare.set); i++ {
			b.set[i] = compare.set[i]
		}
	}
}

// isLenExactMultiple 检查长度是否是字大小的精确倍数
// 返回值:
//   - bool: 如果长度是字大小的精确倍数返回true,否则返回false
func (b *BitSet) isLenExactMultiple() bool {
	return wordsIndex(b.length) == 0
}

// cleanLastWord 通过将未使用的位设置为0来清理最后一个字
func (b *BitSet) cleanLastWord() {
	if !b.isLenExactMultiple() {
		b.set[len(b.set)-1] &= allBits >> (wordSize - wordsIndex(b.length))
	}
}

// Complement 计算位集合的(局部)补集(最多到length位)
// 返回值:
//   - result: *BitSet 补集结果
func (b *BitSet) Complement() (result *BitSet) {
	// 检查参数是否为空
	panicIfNull(b)
	// 创建新的位集合
	result = New(b.length)
	// 计算每个字的补集
	for i, word := range b.set {
		result.set[i] = ^word
	}
	// 清理最后一个字的未使用位
	result.cleanLastWord()
	return
}

// All 检查是否所有位都被设置
// 返回值:
//   - bool: 如果所有位都被设置返回true,否则返回false。空集返回true
func (b *BitSet) All() bool {
	panicIfNull(b)
	return b.Count() == b.length
}

// None 检查是否没有位被设置
// 返回值:
//   - bool: 如果没有位被设置返回true,否则返回false。空集返回true
func (b *BitSet) None() bool {
	panicIfNull(b)
	if b != nil && b.set != nil {
		// 检查每个字是否都为0
		for _, word := range b.set {
			if word > 0 {
				return false
			}
		}
	}
	return true
}

// Any 检查是否有任何位被设置
// 返回值:
//   - bool: 如果有任何位被设置返回true,否则返回false
func (b *BitSet) Any() bool {
	panicIfNull(b)
	return !b.None()
}

// IsSuperSet 检查此集合是否是另一个集合的超集
// 参数:
//   - other: *BitSet 要检查的子集
//
// 返回值:
//   - bool: 如果此集合是other的超集返回true,否则返回false
func (b *BitSet) IsSuperSet(other *BitSet) bool {
	// 获取两个集合中较小的字数
	l := other.wordCount()
	if b.wordCount() < l {
		l = b.wordCount()
	}
	// 检查重叠部分是否包含other的所有位
	for i, word := range other.set[:l] {
		if b.set[i]&word != word {
			return false
		}
	}
	// 检查other多出部分是否都为0
	return popcntSlice(other.set[l:]) == 0
}

// IsStrictSuperSet 检查此集合是否是另一个集合的真超集
// 参数:
//   - other: *BitSet 要检查的子集
//
// 返回值:
//   - bool: 如果此集合是other的真超集返回true,否则返回false
func (b *BitSet) IsStrictSuperSet(other *BitSet) bool {
	return b.Count() > other.Count() && b.IsSuperSet(other)
}

// DumpAsBits 将位集合转储为位字符串
// 按照Go的惯例,最低有效位打印在最后(索引0在字符串末尾)
// 这对于调试和测试很有用。不适合序列化
// 返回值:
//   - string: 位字符串表示
func (b *BitSet) DumpAsBits() string {
	if b.set == nil {
		return "."
	}
	// 创建字符串缓冲区
	buffer := bytes.NewBufferString("")
	// 从高位到低位打印每个字的二进制表示
	i := len(b.set) - 1
	for ; i >= 0; i-- {
		fmt.Fprintf(buffer, "%064b.", b.set[i])
	}
	return buffer.String()
}

// BinaryStorageSize 返回二进制存储需求(参见WriteTo)的字节数
// 返回值:
//   - int: 所需的字节数
func (b *BitSet) BinaryStorageSize() int {
	return int(wordBytes + wordBytes*uint(b.wordCount()))
}

// readUint64Array 从reader中读取uint64数组
// 参数:
//   - reader: io.Reader 读取源
//   - data: []uint64 目标数组
//
// 返回值:
//   - error: 如果发生错误则返回,否则返回nil
func readUint64Array(reader io.Reader, data []uint64) error {
	length := len(data)
	bufferSize := 128
	// 创建读取缓冲区
	buffer := make([]byte, bufferSize*int(wordBytes))
	// 分块读取数据
	for i := 0; i < length; i += bufferSize {
		end := i + bufferSize
		if end > length {
			end = length
			buffer = buffer[:wordBytes*uint(end-i)]
		}
		chunk := data[i:end]
		// 读取一个完整的块
		if _, err := io.ReadFull(reader, buffer); err != nil {
			return err
		}
		// 将字节转换为uint64
		for i := range chunk {
			chunk[i] = uint64(binaryOrder.Uint64(buffer[8*i:]))
		}
	}
	return nil
}

// writeUint64Array 将uint64数组写入writer
// 参数:
//   - writer: io.Writer 写入目标
//   - data: []uint64 源数组
//
// 返回值:
//   - error: 如果发生错误则返回,否则返回nil
func writeUint64Array(writer io.Writer, data []uint64) error {
	bufferSize := 128
	// 创建写入缓冲区
	buffer := make([]byte, bufferSize*int(wordBytes))
	// 分块写入数据
	for i := 0; i < len(data); i += bufferSize {
		end := i + bufferSize
		if end > len(data) {
			end = len(data)
			buffer = buffer[:wordBytes*uint(end-i)]
		}
		chunk := data[i:end]
		// 将uint64转换为字节
		for i, x := range chunk {
			binaryOrder.PutUint64(buffer[8*i:], x)
		}
		// 写入一个完整的块
		_, err := writer.Write(buffer)
		if err != nil {
			return err
		}
	}
	return nil
}

// WriteTo 将BitSet写入流
// 格式为:
// 1. uint64 length
// 2. []uint64 set
// length是BitSet中的位数
//
// set是一个uint64切片,包含length到length+63位
// 默认情况下它被解释为大端序的uint64数组(参见BinaryOrder())
// 这意味着前8位存储在字节索引7,接下来的8位存储在字节索引6...
// 位64到71存储在字节索引8,依此类推
// 如果你改变二进制顺序,需要同时改变读写操作
// 我们建议使用默认的二进制顺序
//
// 成功时返回写入的字节数
//
// 性能提示:如果此函数用于写入磁盘或网络连接,
// 使用bufio.Writer包装流可能会有益处
// 例如:
//
//	f, err := os.Create("myfile")
//	w := bufio.NewWriter(f)
//
// 参数:
//   - stream: io.Writer 写入目标流
//
// 返回值:
//   - int64: 写入的字节数
//   - error: 如果发生错误则返回,否则返回nil
func (b *BitSet) WriteTo(stream io.Writer) (int64, error) {
	length := uint64(b.length)
	// 写入长度
	err := binary.Write(stream, binaryOrder, &length)
	if err != nil {
		// 失败时,我们不保证返回写入的字节数
		return int64(0), err
	}
	// 写入数据
	err = writeUint64Array(stream, b.set[:b.wordCount()])
	if err != nil {
		// 失败时,我们不保证返回写入的字节数
		return int64(wordBytes), err
	}
	return int64(b.BinaryStorageSize()), nil
}

// ReadFrom 从使用WriteTo写入的流中读取BitSet
// 格式为:
// 1. uint64 length
// 2. []uint64 set
// 详见WriteTo
// 成功时返回读取的字节数
// 如果当前BitSet不够大,它会被扩展
// 如果发生错误,BitSet要么保持不变,要么在错误发生太晚无法保留内容时被清空
//
// 性能提示:如果此函数用于从磁盘或网络连接读取,
// 使用bufio.Reader包装流可能会有益处
// 例如:
//
//	f, err := os.Open("myfile")
//	r := bufio.NewReader(f)
//
// 参数:
//   - stream: io.Reader 读取源流
//
// 返回值:
//   - int64: 读取的字节数
//   - error: 如果发生错误则返回,否则返回nil
func (b *BitSet) ReadFrom(stream io.Reader) (int64, error) {
	var length uint64
	// 读取长度
	err := binary.Read(stream, binaryOrder, &length)
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return 0, err
	}
	newlength := uint(length)

	// 检查类型匹配
	if uint64(newlength) != length {
		return 0, errors.New("unmarshalling error: type mismatch")
	}
	// 计算需要的字数
	nWords := wordsNeeded(uint(newlength))
	// 调整切片大小
	if cap(b.set) >= nWords {
		b.set = b.set[:nWords]
	} else {
		b.set = make([]uint64, nWords)
	}

	b.length = newlength

	// 读取数据
	err = readUint64Array(stream, b.set)
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		// 我们不想让BitSet处于部分填充状态,因为这容易出错
		b.set = b.set[:0]
		b.length = 0
		return 0, err
	}

	return int64(b.BinaryStorageSize()), nil
}

// MarshalBinary 将BitSet编码为二进制形式并返回结果
// 详见WriteTo
// 返回值:
//   - []byte: 编码后的字节切片
//   - error: 如果发生错误则返回,否则返回nil
func (b *BitSet) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	_, err := b.WriteTo(&buf)
	if err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), err
}

// UnmarshalBinary 解码由 MarshalBinary 生成的二进制形式
// 详见 WriteTo 方法
// 参数:
//   - data: []byte 要解码的二进制数据
//
// 返回值:
//   - error: 如果发生错误则返回,否则返回nil
func (b *BitSet) UnmarshalBinary(data []byte) error {
	// 创建字节读取器
	buf := bytes.NewReader(data)
	// 调用 ReadFrom 方法读取数据
	_, err := b.ReadFrom(buf)
	return err
}

// MarshalJSON 将位集合编码为 JSON 结构
// 返回值:
//   - []byte: 编码后的 JSON 字节切片
//   - error: 如果发生错误则返回,否则返回nil
func (b BitSet) MarshalJSON() ([]byte, error) {
	// 创建一个缓冲区,预分配足够的空间
	buffer := bytes.NewBuffer(make([]byte, 0, b.BinaryStorageSize()))
	// 将位集合写入缓冲区
	_, err := b.WriteTo(buffer)
	if err != nil {
		return nil, err
	}

	// 使用 base64 编码缓冲区内容,然后转为 JSON
	return json.Marshal(base64Encoding.EncodeToString(buffer.Bytes()))
}

// UnmarshalJSON 从使用 MarshalJSON 创建的 JSON 中解码位集合
// 参数:
//   - data: []byte 要解码的 JSON 数据
//
// 返回值:
//   - error: 如果发生错误则返回,否则返回nil
func (b *BitSet) UnmarshalJSON(data []byte) error {
	// 将 JSON 解码为字符串
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	// 对字符串进行 base64 解码
	buf, err := base64Encoding.DecodeString(s)
	if err != nil {
		return err
	}

	// 从解码后的数据中读取位集合
	_, err = b.ReadFrom(bytes.NewReader(buf))
	return err
}

// Rank 返回位集合中从开始到指定索引(包含)的已设置位的数量
// 参考 https://en.wikipedia.org/wiki/Ranking#Ranking_in_statistics
// 参数:
//   - index: uint 要计算到的索引位置
//
// 返回值:
//   - uint: 已设置位的数量
func (b *BitSet) Rank(index uint) uint {
	// 如果索引超出长度,返回总的已设置位数
	if index >= b.length {
		return b.Count()
	}
	// 计算剩余位数
	leftover := (index + 1) & 63
	// 计算完整字中的已设置位数
	answer := uint(popcntSlice(b.set[:(index+1)>>6]))
	// 处理最后一个不完整字
	if leftover != 0 {
		answer += uint(popcount(b.set[(index+1)>>6] << (64 - leftover)))
	}
	return answer
}

// Select 返回第 j 个已设置位的索引,其中 j 是参数
// 调用者负责确保 0 <= j < Count(): 当 j 超出范围时,函数返回位集合的长度(b.length)
// 注意:此函数与 Rank 函数的约定不同,Rank 函数在对最小值排名时返回 1
// 我们遵循 Select 和 Rank 的传统教科书定义
// 参数:
//   - index: uint 要查找的第几个已设置位
//
// 返回值:
//   - uint: 找到的已设置位的索引
func (b *BitSet) Select(index uint) uint {
	// 记录剩余要找的位数
	leftover := index
	// 遍历每个字
	for idx, word := range b.set {
		// 计算当前字中已设置位的数量
		w := uint(popcount(word))
		// 如果当前字包含目标位
		if w > leftover {
			return uint(idx)*64 + select64(word, leftover)
		}
		// 减去当前字中的已设置位数
		leftover -= w
	}
	return b.length
}

// top 检测最高的已设置位
// 返回值:
//   - uint: 最高已设置位的索引
//   - bool: 是否找到已设置位(true=找到, false=未找到)
func (b *BitSet) top() (uint, bool) {
	panicIfNull(b)

	// 从后向前查找第一个非零字
	idx := len(b.set) - 1
	for ; idx >= 0 && b.set[idx] == 0; idx-- {
	}

	// 如果没有找到已设置位
	if idx < 0 {
		return 0, false
	}

	// 返回最高已设置位的索引
	return uint(idx)*wordSize + len64(b.set[idx]) - 1, true
}

// ShiftLeft 对位集合进行左移操作,类似于 << 运算
// 左移可能需要扩展位集合大小。我们通过检测最左边的已设置位来避免不必要的内存操作
// 如果移位导致超出容量,函数会 panic
// 参数:
//   - bits: uint 要左移的位数
func (b *BitSet) ShiftLeft(bits uint) {
	panicIfNull(b)

	// 如果移位数为0,直接返回
	if bits == 0 {
		return
	}

	// 获取最高已设置位
	top, ok := b.top()
	if !ok {
		return
	}

	// 容量检查
	if top+bits < bits {
		panic("You are exceeding the capacity")
	}

	// 目标集合
	dst := b.set

	// 计算新的大小
	nsize := wordsNeeded(top + bits)
	if len(b.set) < nsize {
		dst = make([]uint64, nsize)
	}
	if top+bits >= b.length {
		b.length = top + bits + 1
	}

	// 计算移位参数
	pad, idx := top%wordSize, top>>log2WordSize
	shift, pages := bits%wordSize, bits>>log2WordSize

	// 执行移位操作
	if bits%wordSize == 0 { // 按整字移位的情况
		copy(dst[pages:nsize], b.set)
	} else { // 需要处理位移位的情况
		if pad+shift >= wordSize {
			dst[idx+pages+1] = b.set[idx] >> (wordSize - shift)
		}

		for i := int(idx); i >= 0; i-- {
			if i > 0 {
				dst[i+int(pages)] = (b.set[i] << shift) | (b.set[i-1] >> (wordSize - shift))
			} else {
				dst[i+int(pages)] = b.set[i] << shift
			}
		}
	}

	// 清零额外的页
	for i := 0; i < int(pages); i++ {
		dst[i] = 0
	}

	b.set = dst
}

// ShiftRight 对位集合进行右移操作,类似于 >> 运算
// 参数:
//   - bits: uint 要右移的位数
func (b *BitSet) ShiftRight(bits uint) {
	panicIfNull(b)

	// 如果移位数为0,直接返回
	if bits == 0 {
		return
	}

	// 获取最高已设置位
	top, ok := b.top()
	if !ok {
		return
	}

	// 如果移位数大于最高位,清空整个集合
	if bits >= top {
		b.set = make([]uint64, wordsNeeded(b.length))
		return
	}

	// 计算移位参数
	pad, idx := top%wordSize, top>>log2WordSize
	shift, pages := bits%wordSize, bits>>log2WordSize

	// 执行移位操作
	if bits%wordSize == 0 { // 按整字移位的情况
		b.set = b.set[pages:]
		b.length -= pages * wordSize
	} else { // 需要处理位移位的情况
		for i := 0; i <= int(idx-pages); i++ {
			if i < int(idx-pages) {
				b.set[i] = (b.set[i+int(pages)] >> shift) | (b.set[i+int(pages)+1] << (wordSize - shift))
			} else {
				b.set[i] = b.set[i+int(pages)] >> shift
			}
		}

		if pad < shift {
			b.set[int(idx-pages)] = 0
		}
	}

	// 清零剩余的字
	for i := int(idx-pages) + 1; i <= int(idx); i++ {
		b.set[i] = 0
	}
}
