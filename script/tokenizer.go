// 包含脚本令牌化的逻辑，用于将脚本分解为可执行的操作码和数据。

package script

import (
	"encoding/binary"
	"fmt"
)

// opcodeArrayRef 用于中断初始化周期。
var opcodeArrayRef *[256]opcode

func init() {
	opcodeArrayRef = &opcodeArray
}

// ScriptTokenizer 提供了一种无需创建分配即可轻松高效地标记交易脚本的工具。
// 每个连续的操作码都使用 Next 函数进行解析，迭代完成后返回 false，这可能是由于成功标记整个脚本或遇到解析错误。
// 在失败的情况下，可以使用Err函数来获取具体的解析错误。
//
// 成功解析操作码后，可以分别通过 Opcode 和 Data 函数获取与其关联的操作码和数据。
//
// ByteIndex 函数可用于获取分词器在原始脚本中的当前偏移量。
type ScriptTokenizer struct {
	script    []byte
	version   uint16
	offset    int32
	opcodePos int32
	op        *opcode
	data      []byte
	err       error
}

// Done 当所有操作码都已用尽或遇到解析失败并且因此状态有关联错误时返回 true。
func (t *ScriptTokenizer) Done() bool {
	return t.err != nil || t.offset >= int32(len(t.script))
}

// Next 尝试解析下一个操作码并返回是否成功。
// 如果在脚本末尾调用、遇到解析失败或由于先前的解析失败而已存在关联错误，则不会成功。
//
// 在返回 true 的情况下，可以使用关联函数获取解析的操作码和数据，并且如果解析了最终操作码，则脚本中的偏移量将指向下一个操作码或脚本的末尾。
//
// 在返回错误的情况下，解析的操作码和数据将是最后成功解析的值（如果有），并且脚本中的偏移量将指向失败的操作码或脚本的末尾（如果调用函数） 当已经在脚本末尾时。
//
// 当已经位于脚本末尾时调用此函数不会被视为错误，只会返回 false。
func (t *ScriptTokenizer) Next() bool {
	if t.Done() {
		return false
	}

	// Increment the op code position each time we attempt to parse the
	// next op code. Note that since the starting value is -1 (no op codes
	// parsed), by incrementing here, we start at 0, then 1, and so on for
	// the other op codes.
	t.opcodePos++

	op := &opcodeArrayRef[t.script[t.offset]]
	switch {
	// No additional data.  Note that some of the opcodes, notably OP_1NEGATE,
	// OP_0, and OP_[1-16] represent the data themselves.
	case op.length == 1:
		t.offset++
		t.op = op
		t.data = nil
		return true

	// Data pushes of specific lengths -- OP_DATA_[1-75].
	case op.length > 1:
		script := t.script[t.offset:]
		if len(script) < op.length {
			t.err = fmt.Errorf("opcode %s requires %d bytes, but script only "+
				"has %d remaining", op.name, op.length, len(script))
			return false
		}

		// Move the offset forward and set the opcode and data accordingly.
		t.offset += int32(op.length)
		t.op = op
		t.data = script[1:op.length]
		return true

	// Data pushes with parsed lengths -- OP_PUSHDATA{1,2,4}.
	case op.length < 0:
		script := t.script[t.offset+1:]
		if len(script) < -op.length {
			t.err = fmt.Errorf("opcode %s requires %d bytes, but script only "+
				"has %d remaining", op.name, -op.length, len(script))
			return false
		}

		// Next -length bytes are little endian length of data.
		var dataLen int32
		switch op.length {
		case -1:
			dataLen = int32(script[0])
		case -2:
			dataLen = int32(binary.LittleEndian.Uint16(script[:2]))
		case -4:
			dataLen = int32(binary.LittleEndian.Uint32(script[:4]))
		default:
			// In practice it should be impossible to hit this
			// check as each op code is predefined, and only uses
			// the specified lengths.
			t.err = fmt.Errorf("invalid opcode length %d", op.length)
			return false
		}

		// Move to the beginning of the data.
		script = script[-op.length:]

		// Disallow entries that do not fit script or were sign extended.
		if dataLen > int32(len(script)) || dataLen < 0 {
			t.err = fmt.Errorf("opcode %s pushes %d bytes, but script only "+
				"has %d remaining", op.name, dataLen, len(script))
			return false
		}

		// Move the offset forward and set the opcode and data accordingly.
		t.offset += 1 + int32(-op.length) + dataLen
		t.op = op
		t.data = script[:dataLen]
		return true
	}

	// The only remaining case is an opcode with length zero which is
	// impossible.
	panic("unreachable")
}

// Script 返回与分词器关联的完整脚本。
func (t *ScriptTokenizer) Script() []byte {
	return t.script
}

// ByteIndex 将当前偏移量返回到接下来将被解析的完整脚本中，因此也暗示了解析之前的所有内容。
func (t *ScriptTokenizer) ByteIndex() int32 {
	return t.offset
}

// OpcodePosition 返回当前操作码计数器。 与上面的 ByteIndex（有时称为程序计数器或 pc）不同，它随着每个节点操作码而递增，并且对于推送数据不会递增多次。
//
// 注意：如果没有解析任何操作码，则返回 -1。
func (t *ScriptTokenizer) OpcodePosition() int32 {
	return t.opcodePos
}

// Opcode 返回与分词器关联的当前操作码。
func (t *ScriptTokenizer) Opcode() byte {
	return t.op.value
}

// Data 返回与最近成功解析的操作码关联的数据。
func (t *ScriptTokenizer) Data() []byte {
	return t.data
}

// Err 返回当前与标记生成器关联的任何错误。 仅当遇到解析错误时，该值才为非零。
func (t *ScriptTokenizer) Err() error {
	return t.err
}

// MakeScriptTokenizer 返回脚本标记生成器的新实例。 传递不受支持的脚本版本将导致返回的标记生成器立即相应地设置错误。
//
// 有关更多详细信息，请参阅 ScriptTokenizer 的文档。
func MakeScriptTokenizer(scriptVersion uint16, script []byte) ScriptTokenizer {
	// 目前仅支持版本 0 脚本。
	var err error
	if scriptVersion != 0 {
		err = fmt.Errorf("script version %d is not supported", scriptVersion)
	}
	return ScriptTokenizer{
		version: scriptVersion,
		script:  script,
		err:     err,
		// 我们在这里使用负值 1，因此第一个操作码的值为 0。
		opcodePos: -1,
	}
}
