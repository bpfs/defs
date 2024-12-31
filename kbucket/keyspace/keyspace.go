package keyspace

import (
	"math/big"
	"sort"

	"github.com/bpfs/defs/utils/logger"
)

// Key 表示 KeySpace 中的标识符。它持有与之关联的 KeySpace 的引用，以及原始标识符和新的 KeySpace 字节。
type Key struct {
	// Space 是与此 Key 相关联的 KeySpace。
	Space KeySpace

	// Original 是标识符的原始值。
	Original []byte

	// Bytes 是标识符在 KeySpace 中的新值。
	Bytes []byte
}

// Equal 判断此 Key 是否与另一个 Key 相等。
//
// 参数:
//   - k2: 要比较的另一个 Key
//
// 返回值:
//   - bool: 如果两个 Key 相等则返回 true，否则返回 false
//
// 如果两个 Key 不在同一个 KeySpace 中，会触发 panic
func (k1 Key) Equal(k2 Key) bool {
	logger.Debugf("比较键的相等性: %x 和 %x", k1.Bytes, k2.Bytes)
	if k1.Space != k2.Space {
		logger.Errorf("键空间不匹配: %v != %v", k1.Space, k2.Space)
		panic("k1 和 k2 不在同一个 KeySpace 中。")
	}
	equal := k1.Space.Equal(k1, k2)
	logger.Debugf("键比较结果: %v", equal)
	return equal
}

// Less 判断此 Key 是否在另一个 Key 之前。
//
// 参数:
//   - k2: 要比较的另一个 Key
//
// 返回值:
//   - bool: 如果此 Key 在 k2 之前则返回 true，否则返回 false
//
// 如果两个 Key 不在同一个 KeySpace 中，会触发 panic
func (k1 Key) Less(k2 Key) bool {
	logger.Debugf("比较键的大小: %x 和 %x", k1.Bytes, k2.Bytes)
	if k1.Space != k2.Space {
		logger.Errorf("键空间不匹配: %v != %v", k1.Space, k2.Space)
		panic("k1 和 k2 不在同一个 KeySpace 中。")
	}
	less := k1.Space.Less(k1, k2)
	logger.Debugf("键大小比较结果: %v", less)
	return less
}

// Distance 计算此 Key 到另一个 Key 的距离。
//
// 参数:
//   - k2: 要计算距离的目标 Key
//
// 返回值:
//   - *big.Int: 表示两个 Key 之间距离的大整数
//
// 如果两个 Key 不在同一个 KeySpace 中，会触发 panic
func (k1 Key) Distance(k2 Key) *big.Int {
	logger.Debugf("计算键之间的距离: %x 和 %x", k1.Bytes, k2.Bytes)
	if k1.Space != k2.Space {
		logger.Errorf("键空间不匹配: %v != %v", k1.Space, k2.Space)
		panic("k1 和 k2 不在同一个 KeySpace 中。")
	}
	dist := k1.Space.Distance(k1, k2)
	logger.Debugf("键距离计算结果: %v", dist)
	return dist
}

// KeySpace 是用于在标识符上执行数学运算的接口。每个 KeySpace 都有自己的属性和规则。参见 XorKeySpace。
type KeySpace interface {
	// Key 将标识符转换为此空间中的 Key。
	//
	// 参数:
	//   - []byte: 要转换的标识符字节切片
	//
	// 返回值:
	//   - Key: 转换后的 Key 对象
	Key([]byte) Key

	// Equal 判断在此 KeySpace 中两个 Key 是否相等。
	//
	// 参数:
	//   - Key: 第一个 Key
	//   - Key: 第二个 Key
	//
	// 返回值:
	//   - bool: 如果两个 Key 相等则返回 true，否则返回 false
	Equal(Key, Key) bool

	// Distance 计算在此 KeySpace 中两个 Key 之间的距离。
	//
	// 参数:
	//   - Key: 第一个 Key
	//   - Key: 第二个 Key
	//
	// 返回值:
	//   - *big.Int: 表示两个 Key 之间距离的大整数
	Distance(Key, Key) *big.Int

	// Less 判断第一个 Key 是否小于第二个 Key。
	//
	// 参数:
	//   - Key: 第一个 Key
	//   - Key: 第二个 Key
	//
	// 返回值:
	//   - bool: 如果第一个 Key 小于第二个 Key 则返回 true，否则返回 false
	Less(Key, Key) bool
}

// byDistanceToCenter 是一个用于按与中心的接近程度对 Keys 进行排序的类型。
type byDistanceToCenter struct {
	Center Key   // 中心 Key，用作距离计算的参考点
	Keys   []Key // 要排序的 Key 列表
}

// Len 返回 Keys 的长度。
//
// 返回值:
//   - int: Keys 切片的长度
func (s byDistanceToCenter) Len() int {
	return len(s.Keys)
}

// Swap 交换 Keys 中的两个元素的位置。
//
// 参数:
//   - i: 第一个元素的索引
//   - j: 第二个元素的索引
func (s byDistanceToCenter) Swap(i, j int) {
	logger.Debugf("交换键的位置: %d 和 %d", i, j)
	s.Keys[i], s.Keys[j] = s.Keys[j], s.Keys[i]
}

// Less 比较 Keys 中的两个元素的距离，判断是否前者距离中心更近。
//
// 参数:
//   - i: 第一个元素的索引
//   - j: 第二个元素的索引
//
// 返回值:
//   - bool: 如果第 i 个元素距离中心更近则返回 true，否则返回 false
func (s byDistanceToCenter) Less(i, j int) bool {
	logger.Debugf("比较键的距离: 索引 %d 和 %d", i, j)
	a := s.Center.Distance(s.Keys[i])
	b := s.Center.Distance(s.Keys[j])
	result := a.Cmp(b) == -1
	logger.Debugf("距离比较结果: %v", result)
	return result
}

// SortByDistance 按照与中心 Key 的距离对 Key 列表进行排序。
//
// 参数:
//   - sp: KeySpace 实例
//   - center: 作为距离计算参考点的中心 Key
//   - toSort: 要排序的 Key 列表
//
// 返回值:
//   - []Key: 按距离排序后的 Key 列表
func SortByDistance(sp KeySpace, center Key, toSort []Key) []Key {
	logger.Debugf("开始按距离排序，中心键: %x，待排序键数量: %d", center.Bytes, len(toSort))
	toSortCopy := make([]Key, len(toSort))
	copy(toSortCopy, toSort)
	bdtc := &byDistanceToCenter{
		Center: center,
		Keys:   toSortCopy,
	}
	sort.Sort(bdtc)
	logger.Debugf("排序完成，返回 %d 个键", len(bdtc.Keys))
	return bdtc.Keys
}
