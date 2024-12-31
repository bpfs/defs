package keyspace

import (
	"bytes"
	"math/big"
	"testing"
)

// TestPrefixLen 测试 ZeroPrefixLen 函数的正确性
// 参数:
//   - t: 测试用例对象，用于报告测试失败
func TestPrefixLen(t *testing.T) {
	// 定义测试用例字节切片数组
	cases := [][]byte{
		{0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00}, // 包含24个前缀零位的字节切片
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // 包含56个前缀零位的字节切片
		{0x00, 0x58, 0xFF, 0x80, 0x00, 0x00, 0xF0}, // 包含9个前缀零位的字节切片
	}
	// 定义期望的前缀零位数数组
	lens := []int{24, 56, 9}

	// 遍历测试用例进行验证
	for i, c := range cases {
		r := ZeroPrefixLen(c) // 计算当前测试用例的前缀零位数
		if r != lens[i] {
			t.Errorf("ZeroPrefixLen failed: %v != %v", r, lens[i]) // 如果计算结果与期望值不符，报告测试失败
		}
	}
}

// TestXorKeySpace 测试 XORKeySpace 的各项功能
// 参数:
//   - t: 测试用例对象，用于报告测试失败
func TestXorKeySpace(t *testing.T) {
	// 定义测试用例字节切片数组
	ids := [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF}, // 全1字节切片
		{0x00, 0x00, 0x00, 0x00}, // 全0字节切片
		{0xFF, 0xFF, 0xFF, 0xF0}, // 末尾为0xF0的字节切片
	}

	// 为每个测试用例生成对应的Key对
	ks := [][2]Key{
		{XORKeySpace.Key(ids[0]), XORKeySpace.Key(ids[0])}, // 使用相同的字节切片生成Key对
		{XORKeySpace.Key(ids[1]), XORKeySpace.Key(ids[1])}, // 使用相同的字节切片生成Key对
		{XORKeySpace.Key(ids[2]), XORKeySpace.Key(ids[2])}, // 使用相同的字节切片生成Key对
	}

	// 测试Key的相等性和生成正确性
	for i, set := range ks {
		if !set[0].Equal(set[1]) {
			t.Errorf("Key not eq. %v != %v", set[0], set[1]) // 验证同一对Key是否相等
		}

		if !bytes.Equal(set[0].Bytes, set[1].Bytes) {
			t.Errorf("Key gen failed. %v != %v", set[0].Bytes, set[1].Bytes) // 验证Key的字节表示是否相等
		}

		if !bytes.Equal(set[0].Original, ids[i]) {
			t.Errorf("ptrs to original. %v != %v", set[0].Original, ids[i]) // 验证Key的原始字节切片是否正确
		}

		if len(set[0].Bytes) != 32 {
			t.Errorf("key length incorrect. 32 != %d", len(set[0].Bytes)) // 验证Key的长度是否为32字节
		}
	}

	// 测试不同Key之间的比较和距离计算
	for i := 1; i < len(ks); i++ {
		if ks[i][0].Less(ks[i-1][0]) == ks[i-1][0].Less(ks[i][0]) {
			t.Errorf("less should be different.") // 验证Less方法的不对称性
		}

		if ks[i][0].Distance(ks[i-1][0]).Cmp(ks[i-1][0].Distance(ks[i][0])) != 0 {
			t.Errorf("distance should be the same.") // 验证距离的对称性
		}

		if ks[i][0].Equal(ks[i-1][0]) {
			t.Errorf("Keys should not be eq. %v != %v", ks[i][0], ks[i-1][0]) // 验证不同Key的不相等性
		}
	}
}

// TestDistancesAndCenterSorting 测试Key之间的距离计算和基于中心点的排序功能
// 参数:
//   - t: 测试用例对象，用于报告测试失败
func TestDistancesAndCenterSorting(t *testing.T) {
	// 定义测试用例字节切片数组
	adjs := [][]byte{
		{173, 149, 19, 27, 192, 183, 153, 192, 177, 175, 71, 127, 177, 79, 207, 38, 166, 169, 247, 96, 121, 228, 139, 240, 144, 172, 183, 232, 54, 123, 253, 14}, // 随机生成的32字节数据
		{223, 63, 97, 152, 4, 169, 47, 219, 64, 87, 25, 45, 196, 61, 215, 72, 234, 119, 138, 220, 82, 188, 73, 140, 232, 5, 36, 192, 20, 184, 17, 25},            // 随机生成的32字节数据
		{73, 176, 221, 176, 149, 143, 22, 42, 129, 124, 213, 114, 232, 95, 189, 154, 18, 3, 122, 132, 32, 199, 53, 185, 58, 157, 117, 78, 52, 146, 157, 127},     // 基准数据
		{73, 176, 221, 176, 149, 143, 22, 42, 129, 124, 213, 114, 232, 95, 189, 154, 18, 3, 122, 132, 32, 199, 53, 185, 58, 157, 117, 78, 52, 146, 157, 127},     // 与基准数据相同
		{73, 176, 221, 176, 149, 143, 22, 42, 129, 124, 213, 114, 232, 95, 189, 154, 18, 3, 122, 132, 32, 199, 53, 185, 58, 157, 117, 78, 52, 146, 157, 126},     // 与基准数据仅最后一位不同
		{73, 0, 221, 176, 149, 143, 22, 42, 129, 124, 213, 114, 232, 95, 189, 154, 18, 3, 122, 132, 32, 199, 53, 185, 58, 157, 117, 78, 52, 146, 157, 127},       // 与基准数据第二个字节不同
	}

	// 将字节切片转换为Key数组
	keys := make([]Key, len(adjs))
	for i, a := range adjs {
		keys[i] = Key{Space: XORKeySpace, Bytes: a}
	}

	// 辅助函数：比较int64和big.Int
	cmp := func(a int64, b *big.Int) int {
		return big.NewInt(a).Cmp(b)
	}

	// 测试相同Key的距离为0
	if cmp(0, keys[2].Distance(keys[3])) != 0 {
		t.Errorf("distance calculation wrong: %v", keys[2].Distance(keys[3]))
	}

	// 测试仅最后一位不同的Key距离为1
	if cmp(1, keys[2].Distance(keys[4])) != 0 {
		t.Errorf("distance calculation wrong: %v", keys[2].Distance(keys[4]))
	}

	// 测试XOR距离计算的正确性
	d1 := keys[2].Distance(keys[5])
	d2 := XOR(keys[2].Bytes, keys[5].Bytes)
	d2 = d2[len(keys[2].Bytes)-len(d1.Bytes()):] // 跳过big.Int的空字节
	if !bytes.Equal(d1.Bytes(), d2) {
		t.Errorf("bytes should be the same. %v == %v", d1.Bytes(), d2)
	}

	// 测试距离大小关系
	if cmp(2<<32, keys[2].Distance(keys[5])) != -1 {
		t.Errorf("2<<32 should be smaller")
	}

	// 测试基于中心点的排序
	keys2 := SortByDistance(XORKeySpace, keys[2], keys)

	// 定义预期的排序顺序
	order := []int{2, 3, 4, 5, 1, 0}

	// 验证排序结果是否符合预期
	for i, o := range order {
		if !bytes.Equal(keys[o].Bytes, keys2[i].Bytes) {
			t.Errorf("order is wrong. %d?? %v == %v", o, keys[o], keys2[i])
		}
	}
}
