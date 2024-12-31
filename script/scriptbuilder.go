// 包含一个构建器，用于以编程方式构建脚本。

package script

import (
	"encoding/binary"
	"fmt"
)

const (
	// defaultScriptAlloc 是用于由 ScriptBuilder 构建的脚本的支持数组的默认大小。
	// 该数组将根据需要动态增长，但这个数字旨在为绝大多数脚本提供足够的空间，而无需多次增长后备数组。
	// 在已知预期脚本大小的情况下，可以使用 WithScriptAllocSize 功能选项进行覆盖。
	defaultScriptAlloc = 500
)

// scriptBuilderConfig 是一个配置结构，可用于修改 ScriptBuilder 的初始化。
type scriptBuilderConfig struct {
	// allocSize 指定脚本构建器的支持数组的初始大小。
	allocSize int
}

// defaultScriptBuilderConfig 返回一个带有默认值设置的新 scriptBuilderConfig。
func defaultScriptBuilderConfig() *scriptBuilderConfig {
	return &scriptBuilderConfig{
		allocSize: defaultScriptAlloc,
	}
}

// ScriptBuilderOpt 是一种函数选项类型，用于修改 ScriptBuilder 的初始化。
type ScriptBuilderOpt func(*scriptBuilderConfig)

// WithScriptAllocSize 指定脚本生成器的支持数组的初始大小。
func WithScriptAllocSize(size int) ScriptBuilderOpt {
	return func(cfg *scriptBuilderConfig) {
		cfg.allocSize = size
	}
}

// ErrScriptNotCanonical 标识非规范脚本。 调用者可以使用类型断言来检测此错误类型。
type ErrScriptNotCanonical string

// Error 实现错误接口。
func (e ErrScriptNotCanonical) Error() string {
	return string(e)
}

// ScriptBuilder 提供了构建自定义脚本的工具。
// 它允许您在遵守规范编码的同时推送操作码、整数和数据。
// 一般来说，它不能确保脚本正确执行，但是任何超出脚本引擎允许的最大限制并因此保证不执行的数据推送都不会被推送，并将导致脚本函数返回错误。
//
// 例如，以下代码将构建一个 2-of-3 多重签名脚本，用于支付脚本哈希（尽管在这种情况下 MultiSigScript() 是生成脚本的更好选择）：
//
//	builder := NewScriptBuilder()
//	builder.AddOp(OP_2).AddData(pubKey1).AddData(pubKey2)
//	builder.AddData(pubKey3).AddOp(OP_3)
//	builder.AddOp(OP_CHECKMULTISIG)
//	script, err := builder.Script()
//	if err != nil {
//		// Handle the error.
//		return
//	}
//	logger.Printf("Final multi-sig script: %x\n", script)
type ScriptBuilder struct {
	script []byte
	err    error
}

// AddOp 将传递的操作码推送到脚本末尾。 如果推送操作码会导致脚本超出允许的最大脚本引擎大小，则不会修改脚本。
func (b *ScriptBuilder) AddOp(opcode byte) *ScriptBuilder {
	if b.err != nil {
		return b
	}

	// Pushes that would cause the script to exceed the largest allowed
	// script size would result in a non-canonical script.
	if len(b.script)+1 > MaxScriptSize {
		b.err = fmt.Errorf("adding an opcode would exceed the maximum "+
			"allowed canonical script length of %d", MaxScriptSize)
		return b
	}

	b.script = append(b.script, opcode)
	return b
}

// AddOps 将传递的操作码推送到脚本的末尾。 如果推送操作码会导致脚本超出允许的最大脚本引擎大小，则不会修改脚本。
func (b *ScriptBuilder) AddOps(opcodes []byte) *ScriptBuilder {
	if b.err != nil {
		return b
	}

	// Pushes that would cause the script to exceed the largest allowed
	// script size would result in a non-canonical script.
	if len(b.script)+len(opcodes) > MaxScriptSize {
		b.err = fmt.Errorf("adding opcodes would exceed the maximum "+
			"allowed canonical script length of %d", MaxScriptSize)
		return b
	}

	b.script = append(b.script, opcodes...)
	return b
}

// canonicalDataSize 返回数据的规范编码将占用的字节数。
func canonicalDataSize(data []byte) int {
	dataLen := len(data)

	// When the data consists of a single number that can be represented
	// by one of the "small integer" opcodes, that opcode will be instead
	// of a data push opcode followed by the number.
	if dataLen == 0 {
		return 1
	} else if dataLen == 1 && data[0] <= 16 {
		return 1
	} else if dataLen == 1 && data[0] == 0x81 {
		return 1
	}

	if dataLen < OP_PUSHDATA1 {
		return 1 + dataLen
	} else if dataLen <= 0xff {
		return 2 + dataLen
	} else if dataLen <= 0xffff {
		return 3 + dataLen
	}

	return 5 + dataLen
}

// addData 是实际将传递的数据推送到脚本末尾的内部函数。
// 它根据数据的长度自动选择规范操作码。
// 零长度缓冲区将导致将空数据推入堆栈（OP_0）。 此功能不强制执行数据限制。
func (b *ScriptBuilder) addData(data []byte) *ScriptBuilder {
	dataLen := len(data)

	// When the data consists of a single number that can be represented
	// by one of the "small integer" opcodes, use that opcode instead of
	// a data push opcode followed by the number.
	if dataLen == 0 || dataLen == 1 && data[0] == 0 {
		b.script = append(b.script, OP_0)
		return b
	} else if dataLen == 1 && data[0] <= 16 {
		b.script = append(b.script, (OP_1-1)+data[0])
		return b
	} else if dataLen == 1 && data[0] == 0x81 {
		b.script = append(b.script, byte(OP_1NEGATE))
		return b
	}

	// Use one of the OP_DATA_# opcodes if the length of the data is small
	// enough so the data push instruction is only a single byte.
	// Otherwise, choose the smallest possible OP_PUSHDATA# opcode that
	// can represent the length of the data.
	if dataLen < OP_PUSHDATA1 {
		b.script = append(b.script, byte((OP_DATA_1-1)+dataLen))
	} else if dataLen <= 0xff {
		b.script = append(b.script, OP_PUSHDATA1, byte(dataLen))
	} else if dataLen <= 0xffff {
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(dataLen))
		b.script = append(b.script, OP_PUSHDATA2)
		b.script = append(b.script, buf...)
	} else {
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(dataLen))
		b.script = append(b.script, OP_PUSHDATA4)
		b.script = append(b.script, buf...)
	}

	// Append the actual data.
	b.script = append(b.script, data...)

	return b
}

// AddFullData 通常不应该由普通用户使用，因为它不包括防止数据推送大于允许的最大大小的检查，从而导致脚本无法执行。
// 这是为了测试目的而提供的，例如故意将大小设置为大于允许的大小的回归测试。
//
// 使用 AddData 代替。
func (b *ScriptBuilder) AddFullData(data []byte) *ScriptBuilder {
	if b.err != nil {
		return b
	}

	return b.addData(data)
}

// AddData 将传递的数据推送到脚本末尾。
// 它根据数据的长度自动选择规范操作码。
// 零长度缓冲区将导致将空数据推送到堆栈 (OP_0)，并且任何大于 MaxScriptElementSize 的数据推送都不会修改脚本，因为脚本引擎不允许这样做。
// 此外，如果推送数据会导致脚本超出脚本引擎允许的最大大小，则不会修改脚本。
func (b *ScriptBuilder) AddData(data []byte) *ScriptBuilder {
	if b.err != nil {
		return b
	}

	// Pushes that would cause the script to exceed the largest allowed
	// script size would result in a non-canonical script.
	dataSize := canonicalDataSize(data)
	if len(b.script)+dataSize > MaxScriptSize {
		b.err = fmt.Errorf("adding %d bytes of data would exceed the "+
			"maximum allowed canonical script length of %d",
			dataSize, MaxScriptSize)
		return b
	}

	// Pushes larger than the max script element size would result in a
	// script that is not canonical.
	dataLen := len(data)
	if dataLen > MaxScriptElementSize {
		b.err = fmt.Errorf("adding a data element of %d bytes would "+
			"exceed the maximum allowed script element size of %d",
			dataLen, MaxScriptElementSize)
		return b
	}

	return b.addData(data)
}

// AddInt64 将传递的整数推送到脚本末尾。
// 如果推送数据会导致脚本超出脚本引擎允许的最大大小，则不会修改脚本。
func (b *ScriptBuilder) AddInt64(val int64) *ScriptBuilder {
	if b.err != nil {
		return b
	}

	// Pushes that would cause the script to exceed the largest allowed
	// script size would result in a non-canonical script.
	if len(b.script)+1 > MaxScriptSize {
		b.err = fmt.Errorf("adding an integer would exceed the "+
			"maximum allow canonical script length of %d",
			MaxScriptSize)
		return b
	}

	// Fast path for small integers and OP_1NEGATE.
	if val == 0 {
		b.script = append(b.script, OP_0)
		return b
	}
	if val == -1 || (val >= 1 && val <= 16) {
		b.script = append(b.script, byte((OP_1-1)+val))
		return b
	}

	return b.AddData(scriptNum(val).Bytes())
}

// Reset 重置脚本，使其没有内容。
func (b *ScriptBuilder) Reset() *ScriptBuilder {
	b.script = b.script[0:0]
	b.err = nil
	return b
}

// 脚本返回当前构建的脚本。
// 当构建脚本时发生任何错误时，脚本将与错误一起返回到第一个错误点。
func (b *ScriptBuilder) Script() ([]byte, error) {
	return b.script, b.err
}

// NewScriptBuilder 返回脚本生成器的新实例。
// 有关详细信息，请参阅 ScriptBuilder。
func NewScriptBuilder(opts ...ScriptBuilderOpt) *ScriptBuilder {
	cfg := defaultScriptBuilderConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return &ScriptBuilder{
		script: make([]byte, 0, cfg.allocSize),
	}
}
