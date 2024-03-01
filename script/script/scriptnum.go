// 实现了脚本数字的处理，这是脚本语言的一个特性。

package script

import "fmt"

const (
	maxInt32 = 1<<31 - 1
	minInt32 = -1 << 31

	// maxScriptNumLen 是大多数操作码可能被解释为整数的最大字节数。
	maxScriptNumLen = 4

	// cltvMaxScriptNumLen 是被解释为整数的最大字节数数据，可以用于由 CHECKLOCKTIMEVERIFY 解释的按时间和按高度锁定。
	//
	// 该值来自以下事实：当前事务锁定时间是 uint32，导致最大锁定时间为 2^32-1（2106 年）。
	// 然而，scriptNum 是有符号的，因此标准的 4 字节 scriptNum 最多只能支持 2^31-1（2038 年）。
	// 因此，需要 5 字节的 scriptNum，因为它将支持最多 2^39-1，这允许日期超出当前锁定时间限制。
	// cltvMaxScriptNumLen = 5
)

// scriptNum 表示脚本引擎中使用的数值，经过特殊处理以处理共识所需的微妙语义。
//
// 所有数字都存储在数据和备用堆栈上，编码为带有符号位的小端字节序。
// 所有数字操作码（例如 OP_ADD、OP_SUB 和 OP_MUL）仅允许对 [-2^31 + 1, 2^31 - 1] 范围内的 4 字节整数进行操作，但是数字操作的结果可能会溢出并保留
// 只要它们不用作其他数字运算的输入或以其他方式解释为整数，则有效。
//
// 例如，OP_ADD 的两个操作数可能为 2^31 - 1，结果为 2^32 - 2，会溢出，但仍会作为加法结果推送到堆栈。
// 然后该值可以用作 OP_VERIFY 的输入，OP_VERIFY 将会成功，因为数据被解释为布尔值。
// 但是，如果将相同的值用作另一个数字操作码（例如 OP_SUB）的输入，则它必须失败。
//
// 该类型通过将所有数值运算结果存储为 int64 来处理溢出来处理上述要求，并提供 Bytes 方法来获取序列化表示（包括溢出的值）。
//
// 然后，每当数据被解释为整数时，都会使用 MakeScriptNum 函数将其转换为这种类型，如果数字超出范围或未根据参数进行最小编码，该函数将返回错误。
// 由于所有数字操作码都涉及从堆栈中提取数据并将其解释为整数，因此它提供了所需的行为。
type scriptNum int64

// checkMinimalDataEncoding 返回传递的字节数组是否符合最小编码要求。
func checkMinimalDataEncoding(v []byte) error {
	if len(v) == 0 {
		return nil
	}

	// Check that the number is encoded with the minimum possible
	// number of bytes.
	//
	// If the most-significant-byte - excluding the sign bit - is zero
	// then we're not minimal.  Note how this test also rejects the
	// negative-zero encoding, [0x80].
	if v[len(v)-1]&0x7f == 0 {
		// One exception: if there's more than one byte and the most
		// significant bit of the second-most-significant-byte is set
		// it would conflict with the sign bit.  An example of this case
		// is +-255, which encode to 0xff00 and 0xff80 respectively.
		// (big-endian).
		if len(v) == 1 || v[len(v)-2]&0x80 == 0 {
			return fmt.Errorf("numeric value encoded as %x is not minimally encoded", v)
		}
	}

	return nil
}

// Bytes 返回序列化为带有符号位的小端字节序的数字。
//
// Example encodings:
//
//	   127 -> [0x7f]
//	  -127 -> [0xff]
//	   128 -> [0x80 0x00]
//	  -128 -> [0x80 0x80]
//	   129 -> [0x81 0x00]
//	  -129 -> [0x81 0x80]
//	   256 -> [0x00 0x01]
//	  -256 -> [0x00 0x81]
//	 32767 -> [0xff 0x7f]
//	-32767 -> [0xff 0xff]
//	 32768 -> [0x00 0x80 0x00]
//	-32768 -> [0x00 0x80 0x80]
func (n scriptNum) Bytes() []byte {
	// Zero encodes as an empty byte slice.
	if n == 0 {
		return nil
	}

	// Take the absolute value and keep track of whether it was originally
	// negative.
	isNegative := n < 0
	if isNegative {
		n = -n
	}

	// Encode to little endian.  The maximum number of encoded bytes is 9
	// (8 bytes for max int64 plus a potential byte for sign extension).
	result := make([]byte, 0, 9)
	for n > 0 {
		result = append(result, byte(n&0xff))
		n >>= 8
	}

	// When the most significant byte already has the high bit set, an
	// additional high byte is required to indicate whether the number is
	// negative or positive.  The additional byte is removed when converting
	// back to an integral and its high bit is used to denote the sign.
	//
	// Otherwise, when the most significant byte does not already have the
	// high bit set, use it to indicate the value is negative, if needed.
	if result[len(result)-1]&0x80 != 0 {
		extraByte := byte(0x00)
		if isNegative {
			extraByte = 0x80
		}
		result = append(result, extraByte)

	} else if isNegative {
		result[len(result)-1] |= 0x80
	}

	return result
}

// Int32 返回限制为有效 int32 的脚本编号。
// 也就是说，当脚本编号高于允许的最大 int32 值时，返回最大 int32 值，反之亦然，返回最小值。
// 请注意，此行为与简单的 int32 转换不同，因为截断和共识规则规定直接转换为 int 的数字提供了此行为。
//
// 实际上，对于大多数操作码，数字永远不应该超出范围，因为它是通过 MakeScriptNum 使用 defaultScriptLen 值创建的，该值会拒绝它们。
// 万一将来最终根据某些算术的结果调用此函数（在被重新解释为整数之前允许超出范围），这将提供正确的行为。
func (n scriptNum) Int32() int32 {
	if n > maxInt32 {
		return maxInt32
	}

	if n < minInt32 {
		return minInt32
	}

	return int32(n)
}

// MakeScriptNum 将传递的序列化字节解释为编码整数，并将结果作为脚本编号返回。
//
// 由于共识规则规定解释为 int 的序列化字节只允许在最大字节数确定的范围内，因此在每个操作码的基础上，当提供的字节导致超出范围的数字时，将返回错误 那个范围的。
// 特别是，绝大多数处理数值的操作码的范围仅限于 4 个字节，因此会将该值传递给此函数，从而产生 [-2^31 + 1, 2^31 - 1] 的允许范围。
//
// 如果对编码的额外检查确定它没有用尽可能小的字节数表示或者是负 0 编码 [0x80]，则 requireMinimal 标志会导致返回错误。
// 例如，考虑数字 127。它可以编码为 [0x7f]、[0x7f 0x00]、[0x7f 0x00 0x00 ...] 等。除了 [0x7f] 之外的所有形式都将在启用 requireMinimal 的情况下返回错误。
//
// scriptNumLen 是在返回 ErrStackNumberTooBig 之前编码值可以达到的最大字节数。 这有效地限制了允许值的范围。
// 警告：如果传递大于 maxScriptNumLen 的值，应格外小心，这可能导致加法和乘法溢出。
//
// 有关示例编码，请参阅 Bytes 函数文档。
func MakeScriptNum(v []byte, requireMinimal bool, scriptNumLen int) (scriptNum, error) {
	// Interpreting data requires that it is not larger than
	// the passed scriptNumLen value.
	if len(v) > scriptNumLen {
		return 0, fmt.Errorf("numeric value encoded as %x is %d bytes which exceeds the max allowed of %d", v, len(v), scriptNumLen)
	}

	// Enforce minimal encoded if requested.
	if requireMinimal {
		if err := checkMinimalDataEncoding(v); err != nil {
			return 0, err
		}
	}

	// Zero is encoded as an empty byte slice.
	if len(v) == 0 {
		return 0, nil
	}

	// Decode from little endian.
	var result int64
	for i, val := range v {
		result |= int64(val) << uint8(8*i)
	}

	// When the most significant byte of the input bytes has the sign bit
	// set, the result is negative.  So, remove the sign bit from the result
	// and make it negative.
	if v[len(v)-1]&0x80 != 0 {
		// The maximum length of v has already been determined to be 4
		// above, so uint8 is enough to cover the max possible shift
		// value of 24.
		result &= ^(int64(0x80) << uint8(8*(len(v)-1)))
		return scriptNum(-result), nil
	}

	return scriptNum(result), nil
}
