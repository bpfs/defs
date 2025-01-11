package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/pb"
)

func init() {
	gob.Register(&pb.HashTable{})
}

// SeparateHashFromData 从数据中分离出SHA-256哈希值和原始数据
//
// 参数:
//   - combinedData []byte: 包含哈希值和原始数据的字节切片
//
// 返回值:
//   - []byte: 分离出的哈希值
//   - []byte: 分离出的原始数据
//   - error: 处理过程中发生的任何错误
func SeparateHashFromData(combinedData []byte) ([]byte, []byte, error) {
	// 检查数据长度是否足够包含SHA-256哈希值
	if len(combinedData) < sha256.Size {
		return nil, nil, fmt.Errorf("数据太短，无法包含有效的SHA-256哈希值")
	}

	// SHA-256哈希值的大小是32字节
	hash := combinedData[:sha256.Size]
	data := combinedData[sha256.Size:]

	return hash, data, nil
}

// MergeFieldsForSigning 接受任意数量和类型的字段，将它们序列化并合并为一个 []byte
//
// 参数:
//   - fields ...interface{}: 任意数量和类型的字段
//
// 返回值:
//   - []byte: 合并后的字节切片
//   - error: 处理过程中发生的任何错误
func MergeFieldsForSigning(fields ...interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)

	// 遍历所有字段并编码
	for _, field := range fields {
		if err := enc.Encode(field); err != nil {
			logger.Errorf("字段编码失败: %v", err)
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

// CalculateFileHash 计算文件的SHA-256 hash
//
// 参数:
//   - file afero.File: 要计算哈希的文件
//
// 返回值:
//   - []byte: 计算得到的SHA-256哈希值
//   - error: 处理过程中发生的任何错误
func CalculateFileHash(file afero.File) ([]byte, error) {
	// 创建一个新的SHA-256哈希实例
	hash := sha256.New()

	// 从文件复制数据到哈希实例
	_, err := io.Copy(hash, file)
	if err != nil {
		logger.Errorf("计算文件的SHA-256 hash失败: %v", err)
		return nil, err
	}

	// 计算并返回哈希值
	return hash.Sum(nil), nil
}

// CalculateHash 计算[]byte的SHA-256 hash值
//
// 参数:
//   - data []byte: 要计算哈希的数据
//
// 返回值:
//   - []byte: 计算得到的SHA-256哈希值
func CalculateHash(data []byte) []byte {
	hash := sha256.New()
	hash.Write(data)
	return hash.Sum(nil)
}

// CompareHashes 比较两个哈希值是否相等
//
// 参数:
//   - hash1 []byte: 第一个哈希值
//   - hash2 []byte: 第二个哈希值
//
// 返回值:
//   - bool: 如果两个哈希值相等，返回 true；否则返回 false
func CompareHashes(hash1, hash2 []byte) bool {
	// 首先比较长度是否相等
	if len(hash1) != len(hash2) {
		return false
	}
	// 逐字节比较
	for i := range hash1 {
		if hash1[i] != hash2[i] {
			return false
		}
	}
	return true
}

// CheckAndMkdir 检查文件夹是否存在，不存在则新建
//
// 参数:
//   - dirPath string: 要检查或创建的目录路径
//
// 返回值:
//   - error: 处理过程中发生的任何错误
func CheckAndMkdir(dirPath string) error {
	// 判断文件夹是否存在
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		// 创建文件夹
		err := os.MkdirAll(dirPath, os.ModePerm)
		if err != nil {
			logger.Errorf("创建文件夹失败: %v", err)
			return err
		}
	}

	return nil
}

// EncodeToBytes 使用 gob 编码将任意数据转换为 []byte
//
// 参数:
//   - data interface{}: 要编码的数据
//
// 返回值:
//   - []byte: 编码后的字节切片
//   - error: 处理过程中发生的任何错误
func EncodeToBytes(data interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)

	// 编码数据
	err := encoder.Encode(data)
	if err != nil {
		logger.Errorf("将任意数据转换为 []byte失败: %v", err)
		return nil, err
	}

	return buffer.Bytes(), nil
}

// DecodeFromBytes 使用 gob 解码将 []byte 转换为指定的数据结构
//
// 参数:
//   - data []byte: 要解码的字节切片
//   - result interface{}: 解码后的结果将存储在这个接口中
//
// 返回值:
//   - error: 处理过程中发生的任何错误
func DecodeFromBytes(data []byte, result interface{}) error {
	buffer := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buffer)

	// 解码数据
	err := decoder.Decode(result)
	if err != nil {
		logger.Errorf("将 []byte 转换为指定的数据结构失败: %v", err)
		return err
	}

	return nil
}

// ToBytes 泛型函数，用于将不同类型的数据转换为 []byte
//
// 参数:
//   - data T: 要转换的数据，类型为 T
//
// 返回值:
//   - []byte: 转换后的字节切片
//   - error: 处理过程中发生的任何错误
func ToBytes[T any](data T) ([]byte, error) {
	var buf bytes.Buffer

	switch v := any(data).(type) {
	case int:
		// 转换 int 为 int64 以确保一致性
		if err := binary.Write(&buf, binary.LittleEndian, int64(v)); err != nil {
			logger.Errorf("转换int为[]byte失败: %v", err)
			return nil, err
		}
	default:
		// 对于其他类型，直接写入
		if err := binary.Write(&buf, binary.LittleEndian, data); err != nil {
			logger.Errorf("转换数据为[]byte失败: %v", err)
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// FromBytes 泛型函数，用于将 []byte 转换回指定类型
//
// 参数:
//   - data []byte: 要转换的字节切片
//
// 返回值:
//   - T: 转换后的数据，类型为 T
//   - error: 处理过程中发生的任何错误
func FromBytes[T any](data []byte) (T, error) {
	var value T
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &value); err != nil {
		logger.Errorf("将[]byte转换为指定类型失败: %v", err)
		return value, err
	}
	return value, nil
}

// JoinStrings 函数将任意数量的字符串参数组合成一个单一的字符串
//
// 参数:
//   - strs ...string: 要组合的字符串列表
//
// 返回值:
//   - string: 组合后的字符串
func JoinStrings(strs ...string) string {
	var escapedStrs []string
	for _, str := range strs {
		// 将字符串中的反斜杠替换成双反斜杠，以转义反斜杠字符
		escapedStr := strings.ReplaceAll(str, "\\", "\\\\")
		// 将字符串中的逗号替换成转义后的逗号，以避免在最终组合的字符串中误解析
		escapedStr = strings.ReplaceAll(escapedStr, ",", "\\,")
		escapedStrs = append(escapedStrs, escapedStr)
	}
	// 使用逗号作为分隔符，将所有处理后的字符串合并成一个字符串
	return strings.Join(escapedStrs, ",")
}

// SplitString 函数将一个组合过的字符串分割成原始的字符串数组
//
// 参数:
//   - combined string: 要分割的组合字符串
//
// 返回值:
//   - []string: 分割后的字符串数组
//   - error: 处理过程中发生的任何错误
func SplitString(combined string) ([]string, error) {
	var result []string
	var segment strings.Builder
	escaped := false // 标记当前字符是否被转义

	for _, char := range combined {
		switch {
		case escaped:
			// 如果前一个字符是转义符（反斜杠），则直接添加字符到当前段
			escaped = false
			segment.WriteRune(char)
		case char == '\\':
			// 遇到反斜杠时，设置转义标志，忽略下一个字符的特殊意义
			escaped = true
		case char == ',':
			// 遇到逗号时，结束当前段，并将其添加到结果中
			result = append(result, segment.String())
			segment.Reset() // 重置字符串构建器以开始新的段
		default:
			// 对于普通字符，直接添加到当前段
			segment.WriteRune(char)
		}
	}
	// 如果最后一个字符是反斜杠，则输入不合法
	if escaped {
		return nil, errors.New("unexpected escape character at the end")
	}
	// 添加最后一个段到结果中（如果有）
	if segment.Len() > 0 {
		result = append(result, segment.String())
	}

	return result, nil
}

// ConvertSliceTableToSortedSlice 将 map[int64]*pb.HashTable 转换为有序的 []*pb.HashTable
func ConvertSliceTableToSortedSlice(sliceTable map[int64]*pb.HashTable) []*pb.HashTable {
	// 创建一个切片来存储所有的键
	keys := make([]int64, 0, len(sliceTable))
	for k := range sliceTable {
		keys = append(keys, k)
	}

	// 对键进行排序
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	// 创建结果切片
	result := make([]*pb.HashTable, 0, len(sliceTable))

	// 按照排序后的键的顺序添加 HashTable 到结果切片
	for _, k := range keys {
		result = append(result, sliceTable[k])
	}

	return result
}

// SerializeSliceTable 将 SliceTable 序列化为字节切片
// 参数:
//   - sliceTable: map[int64]*HashTable 需要序列化的 SliceTable
//
// 返回值:
//   - []byte: 序列化后的字节切片
//   - error: 如果在序列化过程中发生错误，返回相应的错误信息
func SerializeSliceTable(sliceTable map[int64]*pb.HashTable) ([]byte, error) {
	buffer := new(bytes.Buffer)
	encoder := gob.NewEncoder(buffer)
	err := encoder.Encode(sliceTable)
	if err != nil {
		return nil, fmt.Errorf("序列化 SliceTable 失败: %v", err)
	}
	return buffer.Bytes(), nil
}

// DeserializeSliceTable 将字节切片反序列化为 SliceTable
// 参数:
//   - data: []byte 需要反序列化的字节切片
//
// 返回值:
//   - map[int64]*HashTable: 反序列化后的 SliceTable
//   - error: 如果在反序列化过程中发生错误，返回相应的错误信息
func DeserializeSliceTable(data []byte) (map[int64]*pb.HashTable, error) {
	sliceTable := make(map[int64]*pb.HashTable)
	buffer := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buffer)
	err := decoder.Decode(&sliceTable)
	if err != nil {
		return nil, fmt.Errorf("反序列化 SliceTable 失败: %v", err)
	}
	return sliceTable, nil
}

// BoolToByte 将bool转换为byte
// 参数:
//   - b: bool 需要转换的布尔值
//
// 返回值:
//   - byte: 转换后的字节值，true为1，false为0
func BoolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
