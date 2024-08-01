package util

// BitSet 实现一个简单的位集合，用于表示多个布尔值的集合。
// bits 字节切片，用于存储位信息。
type BitSet struct {
	bits []byte
}

// NewBitSet 创建一个新的 BitSet 实例。
// 参数：
//   - size: int 表示位集合的大小。
//
// 返回值：
//   - *BitSet: 新创建的 BitSet 实例。
func NewBitSet(size int) *BitSet {
	return &BitSet{bits: make([]byte, (size+7)/8)}
}

// Set 设置位集合中的某个位为 1。
// 参数：
//   - i: int 表示要设置的位的索引。
func (b *BitSet) Set(i int) {
	b.bits[i/8] |= 1 << (i % 8)
}

// Clear 将位集合中的某个位清零。
// 参数：
//   - i: int 表示要清零的位的索引。
func (b *BitSet) Clear(i int) {
	b.bits[i/8] &^= 1 << (i % 8)
}

// IsSet 检查位集合中的某个位是否为 1。
// 参数：
//   - i: int 表示要检查的位的索引。
//
// 返回值：
//   - bool: 如果该位为 1，返回 true；否则返回 false。
func (b *BitSet) IsSet(i int) bool {
	return (b.bits[i/8] & (1 << (i % 8))) != 0
}

// All 检查位集合中的所有位是否都为 1。
// 返回值：
//   - bool: 如果所有位都为 1，返回 true；否则返回 false。
func (b *BitSet) All() bool {
	if len(b.bits) == 0 {
		return false // BitSet 为空时返回 false
	}
	for i := 0; i < len(b.bits)*8; i++ {
		if !b.IsSet(i) {
			return false
		}
	}
	return true
}

// Any 检查位集合中是否有任意一位为 1。
// 返回值：
//   - bool: 如果有任意一位为 1，返回 true；否则返回 false。
func (b *BitSet) Any() bool {
	for i := 0; i < len(b.bits)*8; i++ {
		if b.IsSet(i) {
			return true
		}
	}
	return false
}

// None 检查位集合中的所有位是否都为 0。
// 返回值：
//   - bool: 如果所有位都为 0，返回 true；否则返回 false。
func (b *BitSet) None() bool {
	for i := 0; i < len(b.bits)*8; i++ {
		if b.IsSet(i) {
			return false
		}
	}
	return true
}
