package script

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	"golang.org/x/crypto/ripemd160"
)

// opcode 定义了一个操作码的结构体。
type opcode struct {
	value  byte                                 // 操作码的值
	name   string                               // 操作码的名称
	length int                                  // 操作码的长度
	opfunc func(*opcode, []byte, *Engine) error // 操作码的执行函数
}

// 这些常量是 btc wiki、比特币核心以及大多数（如果不是所有）其他与处理 BTC 脚本相关的参考资料和软件中使用的官方操作码的值。
const (
	OP_0         = 0x00 // 0 - 表示数字0
	OP_DATA_1    = 0x01 // 1 - 接下来的1个字节是数据
	OP_DATA_2    = 0x02 // 2
	OP_DATA_3    = 0x03 // 3
	OP_DATA_4    = 0x04 // 4
	OP_DATA_5    = 0x05 // 5
	OP_DATA_6    = 0x06 // 6
	OP_DATA_7    = 0x07 // 7
	OP_DATA_8    = 0x08 // 8
	OP_DATA_9    = 0x09 // 9
	OP_DATA_10   = 0x0a // 10
	OP_DATA_11   = 0x0b // 11
	OP_DATA_12   = 0x0c // 12
	OP_DATA_13   = 0x0d // 13
	OP_DATA_14   = 0x0e // 14
	OP_DATA_15   = 0x0f // 15
	OP_DATA_16   = 0x10 // 16
	OP_DATA_17   = 0x11 // 17
	OP_DATA_18   = 0x12 // 18
	OP_DATA_19   = 0x13 // 19
	OP_DATA_20   = 0x14 // 20
	OP_DATA_21   = 0x15 // 21
	OP_DATA_22   = 0x16 // 22
	OP_DATA_23   = 0x17 // 23
	OP_DATA_24   = 0x18 // 24
	OP_DATA_25   = 0x19 // 25
	OP_DATA_26   = 0x1a // 26
	OP_DATA_27   = 0x1b // 27
	OP_DATA_28   = 0x1c // 28
	OP_DATA_29   = 0x1d // 29
	OP_DATA_30   = 0x1e // 30
	OP_DATA_31   = 0x1f // 31
	OP_DATA_32   = 0x20 // 32
	OP_DATA_33   = 0x21 // 33
	OP_DATA_34   = 0x22 // 34
	OP_DATA_35   = 0x23 // 35
	OP_DATA_36   = 0x24 // 36
	OP_DATA_37   = 0x25 // 37
	OP_DATA_38   = 0x26 // 38
	OP_DATA_39   = 0x27 // 39
	OP_DATA_40   = 0x28 // 40
	OP_DATA_41   = 0x29 // 41
	OP_DATA_42   = 0x2a // 42
	OP_DATA_43   = 0x2b // 43
	OP_DATA_44   = 0x2c // 44
	OP_DATA_45   = 0x2d // 45
	OP_DATA_46   = 0x2e // 46
	OP_DATA_47   = 0x2f // 47
	OP_DATA_48   = 0x30 // 48
	OP_DATA_49   = 0x31 // 49
	OP_DATA_50   = 0x32 // 50
	OP_DATA_51   = 0x33 // 51
	OP_DATA_52   = 0x34 // 52
	OP_DATA_53   = 0x35 // 53
	OP_DATA_54   = 0x36 // 54
	OP_DATA_55   = 0x37 // 55
	OP_DATA_56   = 0x38 // 56
	OP_DATA_57   = 0x39 // 57
	OP_DATA_58   = 0x3a // 58
	OP_DATA_59   = 0x3b // 59
	OP_DATA_60   = 0x3c // 60
	OP_DATA_61   = 0x3d // 61
	OP_DATA_62   = 0x3e // 62
	OP_DATA_63   = 0x3f // 63
	OP_DATA_64   = 0x40 // 64
	OP_DATA_65   = 0x41 // 65
	OP_DATA_66   = 0x42 // 66
	OP_DATA_67   = 0x43 // 67
	OP_DATA_68   = 0x44 // 68
	OP_DATA_69   = 0x45 // 69
	OP_DATA_70   = 0x46 // 70
	OP_DATA_71   = 0x47 // 71
	OP_DATA_72   = 0x48 // 72
	OP_DATA_73   = 0x49 // 73
	OP_DATA_74   = 0x4a // 74
	OP_DATA_75   = 0x4b // 75
	OP_1         = 0x51 // 81 - 表示数字1
	OP_PUSHDATA1 = 0x4c // 76 - 接下来的一个字节长度值表示的字节数是数据
	OP_PUSHDATA2 = 0x4d // 77 - 接下来的两个字节长度值表示的字节数是数据
	OP_PUSHDATA4 = 0x4e // 78 - 接下来的四个字节长度值表示的字节数是数据
	OP_1NEGATE   = 0x4f // 79 - 表示数字-1
	OP_DUP       = 0x76 // 118 - 复制栈顶元素
	// OP_RIPEMD160 到 OP_HASH256 为哈希操作相关操作码
	OP_HASH160     = 0xa9 // 169 - 对栈顶元素进行SHA-256然后RIPEMD-160哈希
	OP_CHECKSIG    = 0xac // 172 - 验证交易签名
	OP_EQUALVERIFY = 0x88 // 136 - 相等验证，等同于OP_EQUAL OP_VERIFY组合
)

// Conditional 执行常数。
const (
	OpCondFalse = 0
	OpCondTrue  = 1
	OpCondSkip  = 2
)

// opcodeArray 保存有关所有可能的操作码的详细信息，例如操作码和任何关联数据应占用多少字节、其人类可读的名称以及处理程序函数。
var opcodeArray = [256]opcode{
	// 数据推送操作码
	OP_DATA_1:    {OP_DATA_1, "OP_DATA_1", 2, opcodePushData},
	OP_DATA_2:    {OP_DATA_2, "OP_DATA_2", 3, opcodePushData},
	OP_DATA_3:    {OP_DATA_3, "OP_DATA_3", 4, opcodePushData},
	OP_DATA_4:    {OP_DATA_4, "OP_DATA_4", 5, opcodePushData},
	OP_DATA_5:    {OP_DATA_5, "OP_DATA_5", 6, opcodePushData},
	OP_DATA_6:    {OP_DATA_6, "OP_DATA_6", 7, opcodePushData},
	OP_DATA_7:    {OP_DATA_7, "OP_DATA_7", 8, opcodePushData},
	OP_DATA_8:    {OP_DATA_8, "OP_DATA_8", 9, opcodePushData},
	OP_DATA_9:    {OP_DATA_9, "OP_DATA_9", 10, opcodePushData},
	OP_DATA_10:   {OP_DATA_10, "OP_DATA_10", 11, opcodePushData},
	OP_DATA_11:   {OP_DATA_11, "OP_DATA_11", 12, opcodePushData},
	OP_DATA_12:   {OP_DATA_12, "OP_DATA_12", 13, opcodePushData},
	OP_DATA_13:   {OP_DATA_13, "OP_DATA_13", 14, opcodePushData},
	OP_DATA_14:   {OP_DATA_14, "OP_DATA_14", 15, opcodePushData},
	OP_DATA_15:   {OP_DATA_15, "OP_DATA_15", 16, opcodePushData},
	OP_DATA_16:   {OP_DATA_16, "OP_DATA_16", 17, opcodePushData},
	OP_DATA_17:   {OP_DATA_17, "OP_DATA_17", 18, opcodePushData},
	OP_DATA_18:   {OP_DATA_18, "OP_DATA_18", 19, opcodePushData},
	OP_DATA_19:   {OP_DATA_19, "OP_DATA_19", 20, opcodePushData},
	OP_DATA_20:   {OP_DATA_20, "OP_DATA_20", 21, opcodePushData},
	OP_DATA_21:   {OP_DATA_21, "OP_DATA_21", 22, opcodePushData},
	OP_DATA_22:   {OP_DATA_22, "OP_DATA_22", 23, opcodePushData},
	OP_DATA_23:   {OP_DATA_23, "OP_DATA_23", 24, opcodePushData},
	OP_DATA_24:   {OP_DATA_24, "OP_DATA_24", 25, opcodePushData},
	OP_DATA_25:   {OP_DATA_25, "OP_DATA_25", 26, opcodePushData},
	OP_DATA_26:   {OP_DATA_26, "OP_DATA_26", 27, opcodePushData},
	OP_DATA_27:   {OP_DATA_27, "OP_DATA_27", 28, opcodePushData},
	OP_DATA_28:   {OP_DATA_28, "OP_DATA_28", 29, opcodePushData},
	OP_DATA_29:   {OP_DATA_29, "OP_DATA_29", 30, opcodePushData},
	OP_DATA_30:   {OP_DATA_30, "OP_DATA_30", 31, opcodePushData},
	OP_DATA_31:   {OP_DATA_31, "OP_DATA_31", 32, opcodePushData},
	OP_DATA_32:   {OP_DATA_32, "OP_DATA_32", 33, opcodePushData},
	OP_DATA_33:   {OP_DATA_33, "OP_DATA_33", 34, opcodePushData},
	OP_DATA_34:   {OP_DATA_34, "OP_DATA_34", 35, opcodePushData},
	OP_DATA_35:   {OP_DATA_35, "OP_DATA_35", 36, opcodePushData},
	OP_DATA_36:   {OP_DATA_36, "OP_DATA_36", 37, opcodePushData},
	OP_DATA_37:   {OP_DATA_37, "OP_DATA_37", 38, opcodePushData},
	OP_DATA_38:   {OP_DATA_38, "OP_DATA_38", 39, opcodePushData},
	OP_DATA_39:   {OP_DATA_39, "OP_DATA_39", 40, opcodePushData},
	OP_DATA_40:   {OP_DATA_40, "OP_DATA_40", 41, opcodePushData},
	OP_DATA_41:   {OP_DATA_41, "OP_DATA_41", 42, opcodePushData},
	OP_DATA_42:   {OP_DATA_42, "OP_DATA_42", 43, opcodePushData},
	OP_DATA_43:   {OP_DATA_43, "OP_DATA_43", 44, opcodePushData},
	OP_DATA_44:   {OP_DATA_44, "OP_DATA_44", 45, opcodePushData},
	OP_DATA_45:   {OP_DATA_45, "OP_DATA_45", 46, opcodePushData},
	OP_DATA_46:   {OP_DATA_46, "OP_DATA_46", 47, opcodePushData},
	OP_DATA_47:   {OP_DATA_47, "OP_DATA_47", 48, opcodePushData},
	OP_DATA_48:   {OP_DATA_48, "OP_DATA_48", 49, opcodePushData},
	OP_DATA_49:   {OP_DATA_49, "OP_DATA_49", 50, opcodePushData},
	OP_DATA_50:   {OP_DATA_50, "OP_DATA_50", 51, opcodePushData},
	OP_DATA_51:   {OP_DATA_51, "OP_DATA_51", 52, opcodePushData},
	OP_DATA_52:   {OP_DATA_52, "OP_DATA_52", 53, opcodePushData},
	OP_DATA_53:   {OP_DATA_53, "OP_DATA_53", 54, opcodePushData},
	OP_DATA_54:   {OP_DATA_54, "OP_DATA_54", 55, opcodePushData},
	OP_DATA_55:   {OP_DATA_55, "OP_DATA_55", 56, opcodePushData},
	OP_DATA_56:   {OP_DATA_56, "OP_DATA_56", 57, opcodePushData},
	OP_DATA_57:   {OP_DATA_57, "OP_DATA_57", 58, opcodePushData},
	OP_DATA_58:   {OP_DATA_58, "OP_DATA_58", 59, opcodePushData},
	OP_DATA_59:   {OP_DATA_59, "OP_DATA_59", 60, opcodePushData},
	OP_DATA_60:   {OP_DATA_60, "OP_DATA_60", 61, opcodePushData},
	OP_DATA_61:   {OP_DATA_61, "OP_DATA_61", 62, opcodePushData},
	OP_DATA_62:   {OP_DATA_62, "OP_DATA_62", 63, opcodePushData},
	OP_DATA_63:   {OP_DATA_63, "OP_DATA_63", 64, opcodePushData},
	OP_DATA_64:   {OP_DATA_64, "OP_DATA_64", 65, opcodePushData},
	OP_DATA_65:   {OP_DATA_65, "OP_DATA_65", 66, opcodePushData},
	OP_DATA_66:   {OP_DATA_66, "OP_DATA_66", 67, opcodePushData},
	OP_DATA_67:   {OP_DATA_67, "OP_DATA_67", 68, opcodePushData},
	OP_DATA_68:   {OP_DATA_68, "OP_DATA_68", 69, opcodePushData},
	OP_DATA_69:   {OP_DATA_69, "OP_DATA_69", 70, opcodePushData},
	OP_DATA_70:   {OP_DATA_70, "OP_DATA_70", 71, opcodePushData},
	OP_DATA_71:   {OP_DATA_71, "OP_DATA_71", 72, opcodePushData},
	OP_DATA_72:   {OP_DATA_72, "OP_DATA_72", 73, opcodePushData},
	OP_DATA_73:   {OP_DATA_73, "OP_DATA_73", 74, opcodePushData},
	OP_DATA_74:   {OP_DATA_74, "OP_DATA_74", 75, opcodePushData},
	OP_DATA_75:   {OP_DATA_75, "OP_DATA_75", 76, opcodePushData},
	OP_PUSHDATA1: {OP_PUSHDATA1, "OP_PUSHDATA1", -1, opcodePushData},
	OP_PUSHDATA2: {OP_PUSHDATA2, "OP_PUSHDATA2", -2, opcodePushData},
	OP_PUSHDATA4: {OP_PUSHDATA4, "OP_PUSHDATA4", -4, opcodePushData},
	OP_1NEGATE:   {OP_1NEGATE, "OP_1NEGATE", 1, opcode1Negate},

	// 堆栈操作码
	OP_DUP: {OP_DUP, "OP_DUP", 1, opcodeDup},

	// 加密操作码
	OP_HASH160:  {OP_HASH160, "OP_HASH160", 1, opcodeHash160},
	OP_CHECKSIG: {OP_CHECKSIG, "OP_CHECKSIG", 1, opcodeCheckSig},

	// 按位逻辑操作码。
	OP_EQUALVERIFY: {OP_EQUALVERIFY, "OP_EQUALVERIFY", 1, opcodeEqualVerify},
}

// opcodeOnelineRepls 定义在进行单行反汇编时被替换的操作码名称。 这样做是为了匹配参考实现的输出，同时不更改更好的完整反汇编中的操作码名称。
var opcodeOnelineRepls = map[string]string{
	"OP_1NEGATE": "-1",
	"OP_0":       "0",
	"OP_1":       "1",
	"OP_2":       "2",
	"OP_3":       "3",
	"OP_4":       "4",
	"OP_5":       "5",
	"OP_6":       "6",
	"OP_7":       "7",
	"OP_8":       "8",
	"OP_9":       "9",
	"OP_10":      "10",
	"OP_11":      "11",
	"OP_12":      "12",
	"OP_13":      "13",
	"OP_14":      "14",
	"OP_15":      "15",
	"OP_16":      "16",
}

// *******************************************
// 操作码实现函数从这里开始。
// *******************************************

// opcodePushData 是绝大多数将原始数据（字节）推送到数据堆栈的操作码的通用处理程序。
func opcodePushData(op *opcode, data []byte, vm *Engine) error {
	vm.dstack.PushByteArray(data)
	return nil
}

// opcode1Negate 将编码为数字的 -1 推送到数据堆栈。
func opcode1Negate(op *opcode, data []byte, vm *Engine) error {
	vm.dstack.PushInt(scriptNum(-1))
	return nil
}

// opcodeDup 复制数据堆栈的顶部项目。
//
// Stack transformation: [... x1 x2 x3] -> [... x1 x2 x3 x3]
func opcodeDup(op *opcode, data []byte, vm *Engine) error {
	return vm.dstack.DupN(1)
}

// opcodeHash160 将数据堆栈的顶部项视为原始字节，并将其替换为ripemd160(sha256(data))。
//
// Stack transformation: [... x1] -> [... ripemd160(sha256(x1))]
func opcodeHash160(op *opcode, data []byte, vm *Engine) error {
	buf, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}

	hash := sha256.Sum256(buf)
	vm.dstack.PushByteArray(calcHash(hash[:], ripemd160.New()))
	return nil
}

// opcodeCheckSig 验证交易签名。
// 通常，它从堆栈中取出签名和公钥，并使用事务的相关部分来验证签名。
func opcodeCheckSig(op *opcode, data []byte, vm *Engine) error {
	// 从堆栈中取出签名和公钥。
	signature, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}
	pubKey, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}

	// 使用Engine中定义的加密算法来验证签名。
	// 假设`cryptoAlgo`提供了`VerifySignature`方法来执行实际的验证。
	// 验证函数需要签名、关联的数据（通常是部分事务的哈希）和公钥。
	valid := vm.cryptoAlgo.VerifySignature(signature, data, pubKey)

	// 将验证结果（布尔值）推回堆栈。
	vm.dstack.PushBool(valid)

	return nil
}

// opcodeEqualVerify 是 opcodeEqual 和 opcodeVerify 的组合。
// 具体来说，它删除数据堆栈的顶部 2 项，比较它们，并将结果（编码为布尔值）推回堆栈。
// 然后，它检查数据堆栈顶部的项目作为布尔值，并验证其计算结果是否为 true。 如果不存在，则返回错误。
//
// Stack transformation: [... x1 x2] -> [... bool] -> [...]
func opcodeEqualVerify(op *opcode, data []byte, vm *Engine) error {
	err := opcodeEqual(vm)
	if err == nil {
		err = abstractVerify(op, vm)
	}
	return err
}

// opcodeEqual 删除数据堆栈的前 2 项，将它们作为原始字节进行比较，并将结果（编码为布尔值）推回堆栈。
//
// Stack transformation: [... x1 x2] -> [... bool]
func opcodeEqual(vm *Engine) error {
	a, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}
	b, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}

	vm.dstack.PushBool(bytes.Equal(a, b))
	return nil
}

// disasmOpcode 将所提供的操作码和数据的人类可读反汇编写入所提供的缓冲区中。
// 紧凑标志指示反汇编应该打印更紧凑的数据携带和小整数操作码表示。
// 例如，OP_0 到 OP_16 被替换为数值，并且数据推送仅打印为数据的十六进制表示形式，而不是包括指定要推送的数据量的操作码。
func disasmOpcode(buf *strings.Builder, op *opcode, data []byte, compact bool) {
	// Replace opcode which represent values (e.g. OP_0 through OP_16 and
	// OP_1NEGATE) with the raw value when performing a compact disassembly.
	opcodeName := op.name
	if compact {
		if replName, ok := opcodeOnelineRepls[opcodeName]; ok {
			opcodeName = replName
		}

		// Either write the human-readable opcode or the parsed data in hex for
		// data-carrying opcodes.
		switch {
		case op.length == 1:
			buf.WriteString(opcodeName)

		default:
			buf.WriteString(hex.EncodeToString(data))
		}

		return
	}

	buf.WriteString(opcodeName)

	switch op.length {
	// Only write the opcode name for non-data push opcodes.
	case 1:
		return

	// Add length for the OP_PUSHDATA# opcodes.
	case -1:
		buf.WriteString(fmt.Sprintf(" 0x%02x", len(data)))
	case -2:
		buf.WriteString(fmt.Sprintf(" 0x%04x", len(data)))
	case -4:
		buf.WriteString(fmt.Sprintf(" 0x%08x", len(data)))
	}

	buf.WriteString(fmt.Sprintf(" 0x%02x", data))
}

// AbstractVerify 将数据堆栈的顶部项目作为布尔值进行检查，并验证其计算结果是否为 true。
// 当堆栈上没有项目或该项目计算结果为 false 时，将返回错误。
func abstractVerify(op *opcode, vm *Engine) error {
	verified, err := vm.dstack.PopBool()
	if err != nil {
		return err
	}

	if !verified {
		return fmt.Errorf("%s failed", op.name)
	}
	return nil
}

// calcHash 通过 buf 计算 hasher 的哈希值。
func calcHash(buf []byte, hasher hash.Hash) []byte {
	hasher.Write(buf)
	return hasher.Sum(nil)
}
