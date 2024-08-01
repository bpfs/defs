package util

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
)

// GenerateTaskID 生成任务ID
// 使用时间戳、私钥和随机数生成一个唯一的taskID
// 参数:
//   - ownerPriv *ecdh.PrivateKey: 所有者的私钥
//
// 返回值:
//   - string: 生成的taskID
//   - error: 处理过程中发生的任何错误
func GenerateTaskID(ownerPriv *ecdh.PrivateKey) (string, error) {
	now := time.Now()
	randBigInt, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		logrus.Errorf("[%s]生成随机数时出错: %v", debug.WhereAmI(), err)
		return "", err
	}

	taskID := fmt.Sprintf("%x-%d-%s", ownerPriv.PublicKey.X.Bytes(), now.Unix(), randBigInt.String())
	return taskID, nil
}

// GenerateSegmentID 生成用于文件片段的SegmentID
func GenerateSegmentID(fileID string, index int) (string, error) {
	// 将文件ID和分片索引转换为字节并组合
	input := []byte(fmt.Sprintf("%s-%d", fileID, index))

	// 使用SHA-256对组合后的字节进行哈希，生成SegmentID
	hasher := sha256.New()
	if _, err := hasher.Write(input); err != nil {
		return "", err
	}

	// 将哈希值转换为十六进制字符串作为SegmentID
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// SeparateHashFromData 从数据中分离出SHA-256哈希值和原始数据
func SeparateHashFromData(combinedData []byte) ([]byte, []byte, error) {
	if len(combinedData) < sha256.Size {
		return nil, nil, fmt.Errorf("数据太短，无法包含有效的SHA-256哈希值")
	}

	// SHA-256哈希值的大小是32字节
	hash := combinedData[:sha256.Size]
	data := combinedData[sha256.Size:]

	return hash, data, nil
}

// MergeFieldsForSigning 接受任意数量和类型的字段，将它们序列化并合并为一个 []byte。
func MergeFieldsForSigning(fields ...interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)

	for _, field := range fields {
		if err := enc.Encode(field); err != nil {
			logrus.Errorf("[%s]字段编码失败: %v", debug.WhereAmI(), err)
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

// 计算文件的SHA-256 hash
func CalculateFileHash(file afero.File) ([]byte, error) {
	// New 返回一个新的 hash.Hash 计算 SHA256 校验和。
	hash := sha256.New()

	// Copy 从 src 复制到 dst，直到 src 达到 EOF 或发生错误。
	_, err := io.Copy(hash, file)
	if err != nil {
		logrus.Errorf("[%s]计算文件的SHA-256 hash失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	// Sum 将当前哈希附加到 b 并返回结果切片。 它不会改变底层哈希状态。
	return hash.Sum(nil), nil
}

// 计算[]byte的SHA-256 hash值
func CalculateHash(data []byte) []byte {
	hash := sha256.New()
	hash.Write(data)
	return hash.Sum(nil)
}

// CompareHashes 比较两个哈希值是否相等
// 参数：
//   - hash1: []byte 第一个哈希值
//   - hash2: []byte 第二个哈希值
//
// 返回值：
//   - bool: 如果两个哈希值相等，返回 true；否则返回 false
func CompareHashes(hash1, hash2 []byte) bool {
	if len(hash1) != len(hash2) {
		return false
	}
	for i := range hash1 {
		if hash1[i] != hash2[i] {
			return false
		}
	}
	return true
}

// CheckAndMkdir 检查文件夹是否存在，不存在则新建
func CheckAndMkdir(dirPath string) error {
	// 判断文件夹是否存在
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		// 创建文件夹
		err := os.MkdirAll(dirPath, os.ModePerm)
		if err != nil {
			logrus.Errorf("[%s]创建文件夹失败: %v", debug.WhereAmI(), err)
			return err
		}
	}

	return nil
}

// EncodeToBytes 使用 gob 编码将任意数据转换为 []byte
func EncodeToBytes(data interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)

	err := encoder.Encode(data)
	if err != nil {
		logrus.Errorf("[%s]将任意数据转换为 []byte失败: %v", debug.WhereAmI(), err)
		return nil, err
	}

	return buffer.Bytes(), nil
}

// DecodeFromBytes 使用 gob 解码将 []byte 转换为指定的数据结构
func DecodeFromBytes(data []byte, result interface{}) error {
	buffer := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buffer)

	err := decoder.Decode(result)
	if err != nil {
		logrus.Errorf("[%s]将 []byte 转换为指定的数据结构失败: %v", debug.WhereAmI(), err)
		return err
	}

	return nil
}

// ToBytes 泛型函数，用于将不同类型的数据转换为 []byte
func ToBytes[T any](data T) ([]byte, error) {
	var buf bytes.Buffer

	switch v := any(data).(type) {
	case int:
		// 转换 int 为 int64 以确保一致性
		if err := binary.Write(&buf, binary.LittleEndian, int64(v)); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return nil, err
		}
	default:
		// 对于其他类型，直接写入
		if err := binary.Write(&buf, binary.LittleEndian, data); err != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// FromBytes 泛型函数，用于将 []byte 转换回指定类型
func FromBytes[T any](data []byte) (T, error) {
	var value T
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &value); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return value, err
	}
	return value, nil
}

/**

// 时间转换为 []byte
currentTime := time.Now().Unix() // 将时间转换为 Unix 时间戳
currentTimeBytes, _ := toBytes(currentTime)

// []byte 还原为时间
var recoveredTime int64
recoveredTime, _ = fromBytes[int64](currentTimeBytes)
recoveredTimeObject := time.Unix(recoveredTime, 0) // 从 Unix 时间戳还原为 time.Time

// 布尔值转换为 []byte
boolVal := true
boolBytes, _ := toBytes(boolVal)

// []byte 还原为布尔值
var recoveredBool bool
recoveredBool, _ = fromBytes[bool](boolBytes)

// 数字转换为 []byte
num := float64(1234.56)
numBytes, _ := toBytes(num)

// []byte 还原为数字
var recoveredNum float64
recoveredNum, _ = fromBytes[float64](numBytes)

*/

// func toBytes(data interface{}) ([]byte, error) {
// 	var buf bytes.Buffer
// 	switch v := data.(type) {
// 	case time.Time:
// 		err := binary.Write(&buf, binary.LittleEndian, v.Unix())
// 		if err != nil {
// 			return nil, err
// 		}
// 	case bool:
// 		var boolVal int8
// 		if v {
// 			boolVal = 1
// 		}
// 		err := binary.Write(&buf, binary.LittleEndian, boolVal)
// 		if err != nil {
// 			return nil, err
// 		}
// 	default:
// 		err := binary.Write(&buf, binary.LittleEndian, data)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
// 	return buf.Bytes(), nil
// }

// func fromBytes(data []byte, target interface{}) error {
// 	return binary.Read(bytes.NewReader(data), binary.LittleEndian, target)
// }

/**

// 时间转换为 []byte
currentTime := time.Now()
currentTimeBytes, _ := toBytes(currentTime)

// []byte 还原为 time.Time
var recoveredTime time.Time
_ = fromBytes(currentTimeBytes, &recoveredTime)

// 布尔值转换为 []byte
boolVal := true
boolBytes, _ := toBytes(boolVal)

// []byte 还原为 bool
var recoveredBool bool
_ = fromBytes(boolBytes, &recoveredBool)

// 数字转换为 []byte
num := float64(1234.56)
numBytes, _ := toBytes(num)

// []byte 还原为 float64
var recoveredNum float64
_ = fromBytes(numBytes, &recoveredNum)

*/

// 字节转int
// func bytesToInt(bys []byte) int {
// 	bytebuff := bytes.NewBuffer(bys)
// 	var data int64
// 	binary.Read(bytebuff, binary.BigEndian, &data)
// 	return int(data)
// }

// int64ToBytes 将 int64 转换为 []byte
// func int64ToBytes(i int64) ([]byte, error) {
// 	buf := new(bytes.Buffer)
// 	err := binary.Write(buf, binary.LittleEndian, i)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return buf.Bytes(), nil
// }

// intToBytes 将 int 转换为 []byte
// func intToBytes(i int) ([]byte, error) {
// 	buf := new(bytes.Buffer)
// 	err := binary.Write(buf, binary.LittleEndian, int32(i)) // 如果你的系统是64位，也可以用 int64(i)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return buf.Bytes(), nil
// }

// modeToBytes 将 mode 转换为 []byte
// func modeToBytes(m mode) ([]byte, error) {
// 	buf := new(bytes.Buffer)
// 	err := binary.Write(buf, binary.LittleEndian, int32(m)) // 如果你的系统是64位，也可以用 int64(m)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return buf.Bytes(), nil
// }

// timeToBytes 将 time.Time 转换为 []byte（通过Unix时间戳）
// func timeToBytes(t time.Time) ([]byte, error) {
// 	unixTime := t.Unix()
// 	return int64ToBytes(unixTime)
// }

// bytesToBool 将 []byte 转换为 bool
// func bytesToBool(b []byte) bool {
// 	var flag int8
// 	buffer := bytes.NewBuffer(b)
// 	binary.Read(buffer, binary.LittleEndian, &flag)
// 	return flag != 0
// }

// joinStrings 函数将任意数量的字符串参数组合成一个单一的字符串。
// 这里使用了变长参数，允许函数接受任意数量的字符串。
// 特别地，函数内部对字符串中的逗号和反斜杠进行了转义处理，
// 以确保它们不会影响最终组合字符串的解析。
func JoinStrings(strs ...string) string {
	var escapedStrs []string
	for _, str := range strs {
		// 将字符串中的反斜杠替换成双反斜杠，以转义反斜杠字符。
		escapedStr := strings.ReplaceAll(str, "\\", "\\\\")
		// 将字符串中的逗号替换成转义后的逗号，以避免在最终组合的字符串中误解析。
		escapedStr = strings.ReplaceAll(escapedStr, ",", "\\,")
		escapedStrs = append(escapedStrs, escapedStr)
	}
	// 使用逗号作为分隔符，将所有处理后的字符串合并成一个字符串。
	return strings.Join(escapedStrs, ",")
}

// splitString 函数将一个组合过的字符串分割成原始的字符串数组。
// 这个函数逆转了 joinStrings 函数的操作，正确处理了转义的逗号和反斜杠。
func SplitString(combined string) ([]string, error) {
	var result []string
	var segment strings.Builder
	escaped := false // 标记当前字符是否被转义

	for _, char := range combined {
		switch {
		case escaped:
			// 如果前一个字符是转义符（反斜杠），则直接添加字符到当前段。
			escaped = false
			segment.WriteRune(char)
		case char == '\\':
			// 遇到反斜杠时，设置转义标志，忽略下一个字符的特殊意义。
			escaped = true
		case char == ',':
			// 遇到逗号时，结束当前段，并将其添加到结果中。
			result = append(result, segment.String())
			segment.Reset() // 重置字符串构建器以开始新的段。
		default:
			// 对于普通字符，直接添加到当前段。
			segment.WriteRune(char)
		}
	}
	// 如果最后一个字符是反斜杠，则输入不合法。
	if escaped {
		return nil, errors.New("unexpected escape character at the end")
	}
	// 添加最后一个段到结果中（如果有）。
	if segment.Len() > 0 {
		result = append(result, segment.String())
	}

	return result, nil
}
