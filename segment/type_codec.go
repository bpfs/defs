// Package segment 提供了基本数据类型的编解码功能
// 支持所有Go语言的基本类型，包括整数、浮点数、复数、字符串、字节数组和布尔值
package segment

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
)

// 定义包级别的错误常量
var (
	// ErrInvalidData 表示输入数据太短,无法解码
	ErrInvalidData = errors.New("invalid data: too short")
	// ErrUnsupportedType 表示不支持的数据类型
	ErrUnsupportedType = errors.New("unsupported type")
	// ErrTypeMismatch 表示类型不匹配错误
	ErrTypeMismatch = errors.New("type mismatch")
	// ErrInvalidLength 表示数据长度无效
	ErrInvalidLength = errors.New("invalid data length")
)

// TypeFlag 用于标识数据类型的标记
type TypeFlag byte

// 支持的数据类型标记常量
const (
	// 有符号整数类型
	TypeInt   TypeFlag = iota + 1 // int 类型,占用1字节
	TypeInt8                      // int8 类型,占用1字节
	TypeInt16                     // int16 类型,占用2字节
	TypeInt32                     // int32 类型,占用4字节
	TypeInt64                     // int64 类型,占用8字节

	// 无符号整数类型
	TypeUint   // uint 类型,占用8字节
	TypeUint8  // uint8 类型,占用1字节
	TypeUint16 // uint16 类型,占用2字节
	TypeUint32 // uint32 类型,占用4字节
	TypeUint64 // uint64 类型,占用8字节

	// 浮点数类型
	TypeFloat32 // float32 类型,占用4字节
	TypeFloat64 // float64 类型,占用8字节

	// 字符串和字节类型
	TypeString // string 类型,变长
	TypeBytes  // []byte 类型,变长

	// 布尔类型
	TypeBool // bool 类型,占用1字节

	// 复数类型
	TypeComplex64  // complex64 类型,占用8字节
	TypeComplex128 // complex128 类型,占用16字节
)

// BasicType 定义了所有支持编解码的基本类型
// 包括所有数值类型、字符串、布尔值和字节切片
type BasicType interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 |
		~complex64 | ~complex128 |
		~string | ~bool | ~[]byte
}

// TypeCodec 提供类型编解码功能
// 内部使用对象池来复用字节切片，提高性能
type TypeCodec struct {
	bufferPool sync.Pool // 字节切片对象池，用于减少内存分配
}

// NewTypeCodec 创建一个新的类型编解码器
// 返回值：
//   - *TypeCodec: 类型编解码器实例
func NewTypeCodec() *TypeCodec {
	return &TypeCodec{
		bufferPool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 0, 16)
				return &buf
			},
		},
	}
}

// getBuffer 从对象池获取缓冲区
// 参数:
//   - size: 需要的缓冲区大小
//
// 返回值:
//   - []byte: 获取到的缓冲区
func (c *TypeCodec) getBuffer(size int) []byte {
	buf := c.bufferPool.Get().(*[]byte)
	slice := *buf
	if cap(slice) < size {
		return make([]byte, size)
	}
	return slice[:size]
}

// putBuffer 将缓冲区放回对象池
// 参数:
//   - buf: 要放回的缓冲区
func (c *TypeCodec) putBuffer(buf []byte) {
	if cap(buf) <= 64 { // 只复用较小的缓冲区
		tmp := buf[:0]
		c.bufferPool.Put(&tmp)
	}
}

// Encode 将任意支持的类型编码为字节数组
// 编码格式：第一个字节为类型标记，后续字节为值的二进制表示
//
// 参数：
//   - v: 要编码的值，必须是基本数据类型
//
// 返回值：
//   - []byte: 编码后的字节数组
//   - error: 编码过程中的错误，如果值类型不支持则返回错误
//
// 示例：
//
//	codec := NewTypeCodec()
//	data, err := codec.Encode(123)
func (c *TypeCodec) Encode(v interface{}) ([]byte, error) {
	var typeFlag TypeFlag
	var data []byte

	switch val := v.(type) {
	// 有符号整数
	case int:
		typeFlag = TypeInt
		buf := c.getBuffer(8)
		binary.BigEndian.PutUint64(buf, uint64(val))
		data = buf
	case int8:
		typeFlag = TypeInt8
		data = []byte{byte(val)}
	case int16:
		typeFlag = TypeInt16
		buf := c.getBuffer(2)
		binary.BigEndian.PutUint16(buf, uint16(val))
		data = buf
	case int32:
		typeFlag = TypeInt32
		buf := c.getBuffer(4)
		binary.BigEndian.PutUint32(buf, uint32(val))
		data = buf
	case int64:
		typeFlag = TypeInt64
		buf := c.getBuffer(8)
		binary.BigEndian.PutUint64(buf, uint64(val))
		data = buf

	// 无符号整数
	case uint:
		typeFlag = TypeUint
		buf := c.getBuffer(8)
		binary.BigEndian.PutUint64(buf, uint64(val))
		data = buf
	case uint8:
		typeFlag = TypeUint8
		data = []byte{val}
	case uint16:
		typeFlag = TypeUint16
		buf := c.getBuffer(2)
		binary.BigEndian.PutUint16(buf, val)
		data = buf
	case uint32:
		typeFlag = TypeUint32
		buf := c.getBuffer(4)
		binary.BigEndian.PutUint32(buf, val)
		data = buf
	case uint64:
		typeFlag = TypeUint64
		buf := c.getBuffer(8)
		binary.BigEndian.PutUint64(buf, val)
		data = buf

	// 浮点数
	case float32:
		typeFlag = TypeFloat32
		buf := c.getBuffer(4)
		binary.BigEndian.PutUint32(buf, math.Float32bits(val))
		data = buf
	case float64:
		typeFlag = TypeFloat64
		buf := c.getBuffer(8)
		binary.BigEndian.PutUint64(buf, math.Float64bits(val))
		data = buf

	// 复数
	case complex64:
		typeFlag = TypeComplex64
		buf := c.getBuffer(8)
		binary.BigEndian.PutUint32(buf[:4], math.Float32bits(real(val)))
		binary.BigEndian.PutUint32(buf[4:], math.Float32bits(imag(val)))
		data = buf
	case complex128:
		typeFlag = TypeComplex128
		buf := c.getBuffer(16)
		binary.BigEndian.PutUint64(buf[:8], math.Float64bits(real(val)))
		binary.BigEndian.PutUint64(buf[8:], math.Float64bits(imag(val)))
		data = buf

	// 字符串
	case string:
		typeFlag = TypeString
		data = []byte(val)

	// 字节切片
	case []byte:
		typeFlag = TypeBytes
		data = val

	// 布尔值
	case bool:
		typeFlag = TypeBool
		if val {
			data = []byte{1}
		} else {
			data = []byte{0}
		}

	default:
		return nil, ErrUnsupportedType
	}

	// 组合类型标记和数据
	result := make([]byte, len(data)+1)
	result[0] = byte(typeFlag)
	copy(result[1:], data)

	// 如果使用了缓冲池的数据，需要归还
	if len(data) > 1 && len(data) <= 16 {
		c.putBuffer(data)
	}

	return result, nil
}

// Decode 将字节数组解码为原始类型
// 参数：
//   - data: 要解码的字节数组
//
// 返回值：
//   - interface{}: 解码后的值
//   - error: 解码过程中的错误，如果成功则为 nil
func (c *TypeCodec) Decode(data []byte) (interface{}, error) {
	if len(data) < 1 {
		return nil, ErrInvalidData
	}

	typeFlag := TypeFlag(data[0])
	value := data[1:]

	switch typeFlag {
	// 有符号整数
	case TypeInt:
		if len(value) != 8 {
			return nil, fmt.Errorf("invalid int data length")
		}
		return int(binary.BigEndian.Uint64(value)), nil
	case TypeInt8:
		if len(value) != 1 {
			return nil, fmt.Errorf("invalid int8 data length")
		}
		return int8(value[0]), nil
	case TypeInt16:
		if len(value) != 2 {
			return nil, fmt.Errorf("invalid int16 data length")
		}
		return int16(binary.BigEndian.Uint16(value)), nil
	case TypeInt32:
		if len(value) != 4 {
			return nil, fmt.Errorf("invalid int32 data length")
		}
		return int32(binary.BigEndian.Uint32(value)), nil
	case TypeInt64:
		if len(value) != 8 {
			return nil, fmt.Errorf("invalid int64 data length")
		}
		return int64(binary.BigEndian.Uint64(value)), nil

	// 无符号整数
	case TypeUint:
		if len(value) != 8 {
			return nil, fmt.Errorf("invalid uint data length")
		}
		return uint(binary.BigEndian.Uint64(value)), nil
	case TypeUint8:
		if len(value) != 1 {
			return nil, fmt.Errorf("invalid uint8 data length")
		}
		return value[0], nil
	case TypeUint16:
		if len(value) != 2 {
			return nil, fmt.Errorf("invalid uint16 data length")
		}
		return binary.BigEndian.Uint16(value), nil
	case TypeUint32:
		if len(value) != 4 {
			return nil, fmt.Errorf("invalid uint32 data length")
		}
		return binary.BigEndian.Uint32(value), nil
	case TypeUint64:
		if len(value) != 8 {
			return nil, fmt.Errorf("invalid uint64 data length")
		}
		return binary.BigEndian.Uint64(value), nil

	// 浮点数
	case TypeFloat32:
		if len(value) != 4 {
			return nil, fmt.Errorf("invalid float32 data length")
		}
		return math.Float32frombits(binary.BigEndian.Uint32(value)), nil
	case TypeFloat64:
		if len(value) != 8 {
			return nil, fmt.Errorf("invalid float64 data length")
		}
		return math.Float64frombits(binary.BigEndian.Uint64(value)), nil

	// 复数
	case TypeComplex64:
		if len(value) != 8 {
			return nil, fmt.Errorf("invalid complex64 data length")
		}
		real := math.Float32frombits(binary.BigEndian.Uint32(value[:4]))
		imag := math.Float32frombits(binary.BigEndian.Uint32(value[4:]))
		return complex(real, imag), nil
	case TypeComplex128:
		if len(value) != 16 {
			return nil, fmt.Errorf("invalid complex128 data length")
		}
		real := math.Float64frombits(binary.BigEndian.Uint64(value[:8]))
		imag := math.Float64frombits(binary.BigEndian.Uint64(value[8:]))
		return complex(real, imag), nil

	// 字符串和字节
	case TypeString:
		return string(value), nil
	case TypeBytes:
		return value, nil

	// 布尔值
	case TypeBool:
		if len(value) != 1 {
			return nil, fmt.Errorf("invalid bool data length")
		}
		return value[0] != 0, nil

	default:
		return nil, fmt.Errorf("unknown type flag: %d", typeFlag)
	}
}

// EncodeTo 将值编码到已存在的字节切片中
// 参数：
//   - dst: 目标字节切片
//   - v: 要编码的值
//
// 返回值：
//   - []byte: 编码后的字节切片
//   - error: 编码错误
func (c *TypeCodec) EncodeTo(dst []byte, v interface{}) ([]byte, error) {
	encoded, err := c.Encode(v)
	if err != nil {
		return dst, err
	}

	return append(dst, encoded...), nil
}

// DecodeBytes 专门用于解码字节切片类型
// 参数：
//   - data: 编码的数据
//
// 返回值：
//   - []byte: 解码后的字节切片
//   - error: 解码错误
func (c *TypeCodec) DecodeBytes(data []byte) ([]byte, error) {
	val, err := c.Decode(data)
	if err != nil {
		return nil, err
	}

	if bytes, ok := val.([]byte); ok {
		return bytes, nil
	}
	return nil, ErrTypeMismatch
}

// DecodeTo 将字节数组解码为指定的基本类型
// 参数：
//   - codec: 类型编解码器
//   - data: 要解码的字节数组
//
// 返回值：
//   - T: 解码后的指定类型值
//   - error: 解码错误
func DecodeTo[T BasicType](codec *TypeCodec, data []byte) (T, error) {
	var zero T

	// 先用通用解码获取 interface{}
	val, err := codec.Decode(data)
	if err != nil {
		return zero, err
	}

	// 类型断言
	if typed, ok := val.(T); ok {
		return typed, nil
	}
	return zero, fmt.Errorf("cannot convert to type %T, got %T", zero, val)
}

// MustDecode 是 Decode 的快捷方式，解码失败时会 panic
// 参数：
//   - data: 要解码的字节数组
//
// 返���值：
//   - interface{}: 解码后的值，如果解码失败则 panic
func (c *TypeCodec) MustDecode(data []byte) interface{} {
	v, err := c.Decode(data)
	if err != nil {
		panic(err)
	}
	return v
}

// MustDecodeTo 是 DecodeTo 的快捷方式，解码失败时会 panic
// 参数：
//   - codec: 类型编解码器
//   - data: 要解码的字节数组
//
// 返回值：
//   - T: 解码后的指定类型值，如果解码失败则 panic
func MustDecodeTo[T BasicType](codec *TypeCodec, data []byte) T {
	v, err := DecodeTo[T](codec, data)
	if err != nil {
		panic(err)
	}
	return v
}

// GetType 获取字节数组中存储的值的类型
// 参数：
//   - data: 要检查类型的字节数组
//
// 返回值：
//   - string: 类型名称
//   - error: 获取类型过程中的错误，如果成功则为 nil
func (c *TypeCodec) GetType(data []byte) (string, error) {
	if len(data) < 1 {
		return "", fmt.Errorf("invalid data: too short")
	}

	typeFlag := TypeFlag(data[0])
	switch typeFlag {
	case TypeInt:
		return "int", nil
	case TypeInt8:
		return "int8", nil
	case TypeInt16:
		return "int16", nil
	case TypeInt32:
		return "int32", nil
	case TypeInt64:
		return "int64", nil
	case TypeUint:
		return "uint", nil
	case TypeUint8:
		return "uint8", nil
	case TypeUint16:
		return "uint16", nil
	case TypeUint32:
		return "uint32", nil
	case TypeUint64:
		return "uint64", nil
	case TypeFloat32:
		return "float32", nil
	case TypeFloat64:
		return "float64", nil
	case TypeComplex64:
		return "complex64", nil
	case TypeComplex128:
		return "complex128", nil
	case TypeString:
		return "string", nil
	case TypeBytes:
		return "[]byte", nil
	case TypeBool:
		return "bool", nil
	default:
		return "", fmt.Errorf("unknown type flag: %d", typeFlag)
	}
}
