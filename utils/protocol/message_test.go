package protocol

import (
	"bytes"
	"encoding/hex"
	"hash/crc32"
	"testing"
	"time"
)

func TestMessageWrapper_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		message *MessageWrapper
	}{
		{
			name: "基本消息",
			message: &MessageWrapper{
				Version:   CurrentVersion,
				Timestamp: time.Now().UnixNano(),
				Sequence:  1,
				Payload:   []byte("test payload"),
			},
		},
		{
			name: "空负载消息",
			message: &MessageWrapper{
				Version:   CurrentVersion,
				Timestamp: time.Now().UnixNano(),
				Sequence:  2,
				Payload:   []byte{},
			},
		},
		{
			name: "大负载消息",
			message: &MessageWrapper{
				Version:   CurrentVersion,
				Timestamp: time.Now().UnixNano(),
				Sequence:  3,
				Payload:   bytes.Repeat([]byte("large payload "), 100),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 序列化
			data, err := tt.message.Marshal()
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// 打印序列化后的数据
			t.Logf("序列化数据: len=%d\n头部: %s\n校验和范围: %s",
				len(data),
				hex.EncodeToString(data[:22]),
				hex.EncodeToString(data[6:min(56, uint32(len(data)))]))

			// 反序列化
			decoded := &MessageWrapper{}
			if err := decoded.Unmarshal(data); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			// 验证字段
			if decoded.Version != tt.message.Version {
				t.Errorf("Version mismatch: got %v, want %v", decoded.Version, tt.message.Version)
			}
			if decoded.Checksum != tt.message.Checksum {
				t.Errorf("Checksum mismatch: got %v, want %v", decoded.Checksum, tt.message.Checksum)
			}
			if decoded.Timestamp != tt.message.Timestamp {
				t.Errorf("Timestamp mismatch: got %v, want %v", decoded.Timestamp, tt.message.Timestamp)
			}
			if decoded.Sequence != tt.message.Sequence {
				t.Errorf("Sequence mismatch: got %v, want %v", decoded.Sequence, tt.message.Sequence)
			}
			if !bytes.Equal(decoded.Payload, tt.message.Payload) {
				t.Errorf("Payload mismatch: got %v, want %v", decoded.Payload, tt.message.Payload)
			}
		})
	}
}

func TestMessageWrapper_ChecksumValidation(t *testing.T) {
	// 创建原始消息
	original := &MessageWrapper{
		Version:   CurrentVersion,
		Timestamp: time.Now().UnixNano(),
		Sequence:  1,
		Payload:   []byte("test payload"),
	}

	// 序列化
	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// 修改数据的不同部分并验证校验和
	tests := []struct {
		name        string
		modifyData  func([]byte)
		shouldFail  bool
		description string
	}{
		{
			name: "修改版本号",
			modifyData: func(data []byte) {
				data[0] = 0xFF // 修改版本号
			},
			shouldFail:  false, // 不应该失败，因为版本号不在校验和范围内
			description: "修改版本号不应影响校验和",
		},
		{
			name: "修改时间戳",
			modifyData: func(data []byte) {
				data[6] = 0xFF // 修改时间戳
			},
			shouldFail:  true,
			description: "修改时间戳应导致校验和错误",
		},
		{
			name: "修改负载",
			modifyData: func(data []byte) {
				data[22] = 0xFF // 修改负载
			},
			shouldFail:  true,
			description: "修改负载应导致校验和错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 复制原始数据
			modifiedData := make([]byte, len(data))
			copy(modifiedData, data)

			// 记录修改前的校验和
			t.Logf("修改前数据: [% x]", modifiedData[6:min(50, uint32(len(modifiedData)))])
			originalDataChecksum := crc32.ChecksumIEEE(modifiedData[6:])

			// 修改数据
			tt.modifyData(modifiedData)

			// 记录修改后的校验和
			t.Logf("修改后数据: [% x]", modifiedData[6:min(50, uint32(len(modifiedData)))])
			modifiedDataChecksum := crc32.ChecksumIEEE(modifiedData[6:])

			// 尝试解析修改后的数据
			decoded := &MessageWrapper{}
			err := decoded.Unmarshal(modifiedData)

			if tt.shouldFail {
				if err == nil {
					t.Errorf("期望校验和错误但成功解析: %s\n原始校验和=%d\n修改后校验和=%d",
						tt.description, originalDataChecksum, modifiedDataChecksum)
				}
			} else {
				if err != nil {
					t.Errorf("不期望出现错误但失败: %v - %s", err, tt.description)
				}
			}
		})
	}
}
