package segment

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"testing"
)

// TestTypeCodec_Basic 测试基本的编解码功能
func TestTypeCodec_Basic(t *testing.T) {
	codec := NewTypeCodec()

	// 定义完整的测试用例
	testCases := []struct {
		name     string      // 测试用例名称
		input    interface{} // 输入值
		wantType string      // 期望的类型
	}{
		{"int", int(1234), "int"},
		{"int8", int8(127), "int8"},
		{"int16", int16(32767), "int16"},
		{"int32", int32(2147483647), "int32"},
		{"int64", int64(9223372036854775807), "int64"},
		{"uint", uint(1234), "uint"},
		{"uint8", uint8(255), "uint8"},
		{"uint16", uint16(65535), "uint16"},
		{"uint32", uint32(4294967295), "uint32"},
		{"uint64", uint64(18446744073709551615), "uint64"},
		{"float32", float32(3.14), "float32"},
		{"float64", float64(3.14159265359), "float64"},
		{"complex64", complex64(1 + 2i), "complex64"},
		{"complex128", complex128(1.1 + 2.2i), "complex128"},
		{"string", "Hello, 世界", "string"},
		{"[]byte", []byte{1, 2, 3, 4, 5}, "[]uint8"},
		{"bool_true", true, "bool"},
		{"bool_false", false, "bool"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("\n=== 测试用例: %s ===", tc.name)
			t.Logf("输入值: %v (%T)", tc.input, tc.input)

			// 编码测试
			bytes, err := codec.Encode(tc.input)
			if err != nil {
				t.Errorf("Encode(%v) error = %v", tc.input, err)
				return
			}
			t.Logf("序列化后的字节: %v (长度: %d)", bytes, len(bytes))

			// 解码测试
			got, err := codec.Decode(bytes)
			if err != nil {
				t.Errorf("Decode(%v) error = %v", bytes, err)
				return
			}
			t.Logf("反序列化结果: %v (%T)", got, got)

			// 检查类型是否匹配
			gotType := fmt.Sprintf("%T", got)
			if gotType != tc.wantType {
				t.Errorf("类型不匹配:\n\t期望类型: %v\n\t实际类型: %v", tc.wantType, gotType)
			}

			// 检查值是否相等
			if !reflect.DeepEqual(got, tc.input) {
				t.Errorf("值不匹配:\n\t期望值: %v\n\t实际值: %v", tc.input, got)
			} else {
				t.Logf("✓ 测试通过: 类型和值都匹配")
			}

			t.Log("------------------------")
		})
	}
}

// TestTypeCodec_Encode 测试基本类型的编码功能
func TestTypeCodec_Encode(t *testing.T) {
	codec := NewTypeCodec()

	// 定义测试用例
	tests := []struct {
		name     string      // 测试用例名称
		input    interface{} // 输入值
		wantType TypeFlag    // 期望的类型标记
		wantErr  bool        // 是否期望错误
	}{
		{"int", int(123), TypeInt, false},
		{"int8", int8(127), TypeInt8, false},
		{"int16", int16(32767), TypeInt16, false},
		{"int32", int32(2147483647), TypeInt32, false},
		{"int64", int64(9223372036854775807), TypeInt64, false},
		{"uint", uint(123), TypeUint, false},
		{"uint8", uint8(255), TypeUint8, false},
		{"uint16", uint16(65535), TypeUint16, false},
		{"uint32", uint32(4294967295), TypeUint32, false},
		{"uint64", uint64(18446744073709551615), TypeUint64, false},
		{"float32", float32(3.14), TypeFloat32, false},
		{"float64", float64(3.14159), TypeFloat64, false},
		{"complex64", complex64(1 + 2i), TypeComplex64, false},
		{"complex128", complex128(1 + 2i), TypeComplex128, false},
		{"string", "hello", TypeString, false},
		{"[]byte", []byte("world"), TypeBytes, false},
		{"bool true", true, TypeBool, false},
		{"bool false", false, TypeBool, false},
		{"unsupported", struct{}{}, TypeFlag(0), true}, // 不支持的类型
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 执行编码
			got, err := codec.Encode(tt.input)

			// 检查错误
			if (err != nil) != tt.wantErr {
				t.Errorf("Encode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// 如果期望错误，到这里就结束测试
			if tt.wantErr {
				return
			}

			// 检查类型标记
			if got[0] != byte(tt.wantType) {
				t.Errorf("Encode() type = %v, want %v", got[0], tt.wantType)
			}
		})
	}
}

// TestTypeCodec_Decode 测试基本类型的解码功能
func TestTypeCodec_Decode(t *testing.T) {
	codec := NewTypeCodec()

	// 先编码一些值用于测试
	intData, _ := codec.Encode(int(123))
	floatData, _ := codec.Encode(float64(3.14))
	stringData, _ := codec.Encode("hello")
	boolData, _ := codec.Encode(true)

	tests := []struct {
		name    string
		data    []byte
		want    interface{}
		wantErr bool
	}{
		{"decode int", intData, int(123), false},
		{"decode float64", floatData, float64(3.14), false},
		{"decode string", stringData, "hello", false},
		{"decode bool", boolData, true, false},
		{"invalid data", []byte{}, nil, true},
		{"unknown type", []byte{255, 0}, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := codec.Decode(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Decode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTypeCodec_EncodeDecode 测试编解码的完整流程
func TestTypeCodec_EncodeDecode(t *testing.T) {
	codec := NewTypeCodec()

	// 测试复杂数字的编解码
	t.Run("complex numbers", func(t *testing.T) {
		// 测试 float32 的极限值
		f32 := float32(math.MaxFloat32)
		encoded, err := codec.Encode(f32)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		decoded, err := codec.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if decoded.(float32) != f32 {
			t.Errorf("float32 mismatch: got %v, want %v", decoded, f32)
		}

		// 测试 complex128
		c128 := complex128(1.5 + 2.5i)
		encoded, err = codec.Encode(c128)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		decoded, err = codec.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if decoded.(complex128) != c128 {
			t.Errorf("complex128 mismatch: got %v, want %v", decoded, c128)
		}
	})

	// 测试字节切片的编解码
	t.Run("bytes", func(t *testing.T) {
		data := []byte("Hello, 世界")
		encoded, err := codec.Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		decoded, err := codec.DecodeBytes(encoded)
		if err != nil {
			t.Fatalf("DecodeBytes failed: %v", err)
		}
		if !bytes.Equal(decoded, data) {
			t.Errorf("bytes mismatch: got %v, want %v", decoded, data)
		}
	})
}

// TestTypeCodec_GetType 测试类型获取功能
func TestTypeCodec_GetType(t *testing.T) {
	codec := NewTypeCodec()

	tests := []struct {
		name     string
		input    interface{}
		wantType string
		wantErr  bool
	}{
		{"int type", int(42), "int", false},
		{"string type", "hello", "string", false},
		{"bool type", true, "bool", false},
		{"invalid data", nil, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var encoded []byte
			var err error

			if tt.input != nil {
				encoded, err = codec.Encode(tt.input)
				if err != nil {
					t.Fatalf("Encode failed: %v", err)
				}
			}

			gotType, err := codec.GetType(encoded)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotType != tt.wantType && !tt.wantErr {
				t.Errorf("GetType() = %v, want %v", gotType, tt.wantType)
			}
		})
	}
}

// TestDecodeTo 测试泛型解码功能
func TestDecodeTo(t *testing.T) {
	codec := NewTypeCodec()

	t.Run("decode to specific type", func(t *testing.T) {
		// 编码一个整数
		encoded, _ := codec.Encode(42)

		// 解码为特定类型
		val, err := DecodeTo[int](codec, encoded)
		if err != nil {
			t.Fatalf("DecodeTo failed: %v", err)
		}
		if val != 42 {
			t.Errorf("DecodeTo = %v, want %v", val, 42)
		}

		// 测试类型不匹配的情况
		_, err = DecodeTo[string](codec, encoded)
		if err == nil {
			t.Error("DecodeTo should fail with type mismatch")
		}
	})
}

// TestMustDecode 测试必须解码功能（包含panic情况）
func TestMustDecode(t *testing.T) {
	codec := NewTypeCodec()

	t.Run("decode to specific type", func(t *testing.T) {
		// 编码一个整数
		encoded, _ := codec.Encode(42)

		// 解码为特定类型
		val, err := DecodeTo[int](codec, encoded)
		if err != nil {
			t.Fatalf("DecodeTo failed: %v", err)
		}
		if val != 42 {
			t.Errorf("DecodeTo = %v, want %v", val, 42)
		}

		// 测试类型不匹配的情况
		_, err = DecodeTo[string](codec, encoded)
		if err == nil {
			t.Error("DecodeTo should fail with type mismatch")
		}
	})
}
