// Copyright 2019 Tim Shannon. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package badgerhold

import (
	"bytes"
	"encoding/gob"
)

// EncodeFunc 是一个用于将值编码为字节的函数类型
type EncodeFunc func(value interface{}) ([]byte, error)

// DecodeFunc 是一个用于从字节解码值的函数类型
type DecodeFunc func(data []byte, value interface{}) error

// DefaultEncode 是 badgerhold 的默认编码函数，使用 Gob 编码
// 参数:
//   - value: interface{} 类型，表示要编码的值
//
// 返回值：
//   - []byte: 编码后的字节切片
//   - error: 如果编码过程中出现错误，则返回错误信息
func DefaultEncode(value interface{}) ([]byte, error) {
	var buff bytes.Buffer // 创建一个字节缓冲区，用于存储编码后的数据

	en := gob.NewEncoder(&buff) // 创建一个 Gob 编码器，将数据编码到缓冲区中

	// 使用编码器对值进行编码
	err := en.Encode(value)
	if err != nil {
		// 如果编码过程中出现错误，记录日志并返回错误信息
		logger.Error("Gob编码失败", "错误", err)
		return nil, err
	}

	// 返回编码后的字节切片
	return buff.Bytes(), nil
}

// DefaultDecode 是 badgerhold 的默认解码函数，使用 Gob 解码
// 参数:
//   - data: []byte 类型，表示要解码的字节数据
//   - value: interface{} 类型，表示要解码到的目标值
//
// 返回值：
//   - error: 如果解码过程中出现错误，则返回错误信息
func DefaultDecode(data []byte, value interface{}) error {
	var buff bytes.Buffer       // 创建一个字节缓冲区，用于存储待解码的数据
	de := gob.NewDecoder(&buff) // 创建一个 Gob 解码器，从缓冲区中解码数据

	// 将字节数据写入缓冲区
	_, err := buff.Write(data)
	if err != nil {
		// 如果写入过程中出现错误，记录日志并返回错误信息
		logger.Error("写入缓冲区失败", "错误", err)
		return err
	}

	// 使用解码器对缓冲区中的数据进行解码，并将结果存储到 value 中
	err = de.Decode(value)
	if err != nil {
		logger.Error("Gob解码失败", "错误", err)
		return err
	}
	return nil
}

// encodeKey 对键值进行编码，并添加类型前缀，允许在 badger DB 中存在多种不同类型
// 参数:
//   - key: interface{} 类型，表示要编码的键值
//   - typeName: string 类型，表示键值所属的数据类型名称
//
// 返回值：
//   - []byte: 编码后的字节切片，包含类型前缀和编码后的键值
//   - error: 如果编码过程中出现错误，则返回错误信息
func (s *Store) encodeKey(key interface{}, typeName string) ([]byte, error) {
	// 调用 Store 的 encode 方法对键值进行编码
	encoded, err := s.encode(key)
	if err != nil {
		// 如果编码过程中出现错误，记录日志并返回错误信息
		logger.Error("键值编码失败", "错误", err)
		return nil, err
	}

	// 返回包含类型前缀和编码后的键值的字节切片
	return append(typePrefix(typeName), encoded...), nil
}

// decodeKey 对键值进行解码，并移除类型前缀
// 参数:
//   - data: []byte 类型，表示包含类型前缀和编码后键值的字节切片
//   - key: interface{} 类型，表示解码后的键值将存储到此变量中
//   - typeName: string 类型，表示键值所属的数据类型名称
//
// 返回值：
//   - error: 如果解码过程中出现错误，则返回错误信息
func (s *Store) decodeKey(data []byte, key interface{}, typeName string) error {
	// 调用 Store 的 decode 方法，对移除类型前缀后的字节数据进行解码
	err := s.decode(data[len(typePrefix(typeName)):], key)
	if err != nil {
		logger.Error("键值解码失败", "错误", err)
		return err
	}
	return nil
}
