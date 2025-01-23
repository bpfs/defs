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

// 这些常量是比特币脚本中使用的标准操作码。
// 每个常量代表一个特定的操作或指令。
const (
	OP_0           = 0x00 // 将数字0推入栈
	OP_DATA_1      = 0x01 // 将接下来的1个字节作为数据推入栈
	OP_DATA_2      = 0x02 // 将接下来的2个字节作为数据推入栈
	OP_DATA_3      = 0x03 // 将接下来的3个字节作为数据推入栈
	OP_DATA_4      = 0x04 // 将接下来的4个字节作为数据推入栈
	OP_DATA_5      = 0x05 // 将接下来的5个字节作为数据推入栈
	OP_DATA_6      = 0x06 // 将接下来的6个字节作为数据推入栈
	OP_DATA_7      = 0x07 // 将接下来的7个字节作为数据推入栈
	OP_DATA_8      = 0x08 // 将接下来的8个字节作为数据推入栈
	OP_DATA_9      = 0x09 // 将接下来的9个字节作为数据推入栈
	OP_DATA_10     = 0x0a // 将接下来的10个字节作为数据推入栈
	OP_DATA_11     = 0x0b // 将接下来的11个字节作为数据推入栈
	OP_DATA_12     = 0x0c // 将接下来的12个字节作为数据推入栈
	OP_DATA_13     = 0x0d // 将接下来的13个字节作为数据推入栈
	OP_DATA_14     = 0x0e // 将接下来的14个字节作为数据推入栈
	OP_DATA_15     = 0x0f // 将接下来的15个字节作为数据推入栈
	OP_DATA_16     = 0x10 // 将接下来的16个字节作为数据推入栈
	OP_DATA_17     = 0x11 // 将接下来的17个字节作为数据推入栈
	OP_DATA_18     = 0x12 // 将接下来的18个字节作为数据推入栈
	OP_DATA_19     = 0x13 // 将接下来的19个字节作为数据推入栈
	OP_DATA_20     = 0x14 // 将接下来的20个字节作为数据推入栈
	OP_DATA_21     = 0x15 // 将接下来的21个字节作为数据推入栈
	OP_DATA_22     = 0x16 // 将接下来的22个字节作为数据推入栈
	OP_DATA_23     = 0x17 // 将接下来的23个字节作为数据推入栈
	OP_DATA_24     = 0x18 // 将接下来的24个字节作为数据推入栈
	OP_DATA_25     = 0x19 // 将接下来的25个字节作为数据推入栈
	OP_DATA_26     = 0x1a // 将接下来的26个字节作为数据推入栈
	OP_DATA_27     = 0x1b // 将接下来的27个字节作为数据推入栈
	OP_DATA_28     = 0x1c // 将接下来的28个字节作为数据推入栈
	OP_DATA_29     = 0x1d // 将接下来的29个字节作为数据推入栈
	OP_DATA_30     = 0x1e // 将接下来的30个字节作为数据推入栈
	OP_DATA_31     = 0x1f // 将接下来的31个字节作为数据推入栈
	OP_DATA_32     = 0x20 // 将接下来的32个字节作为数据推入栈
	OP_DATA_33     = 0x21 // 将接下来的33个字节作为数据推入栈
	OP_DATA_34     = 0x22 // 将接下来的34个字节作为数据推入栈
	OP_DATA_35     = 0x23 // 将接下来的35个字节作为数据推入栈
	OP_DATA_36     = 0x24 // 将接下来的36个字节作为数据推入栈
	OP_DATA_37     = 0x25 // 将接下来的37个字节作为数据推入栈
	OP_DATA_38     = 0x26 // 将接下来的38个字节作为数据推入栈
	OP_DATA_39     = 0x27 // 将接下来的39个字节作为数据推入栈
	OP_DATA_40     = 0x28 // 将接下来的40个字节作为数据推入栈
	OP_DATA_41     = 0x29 // 将接下来的41个字节作为数据推入栈
	OP_DATA_42     = 0x2a // 将接下来的42个字节作为数据推入栈
	OP_DATA_43     = 0x2b // 将接下来的43个字节作为数据推入栈
	OP_DATA_44     = 0x2c // 将接下来的44个字节作为数据推入栈
	OP_DATA_45     = 0x2d // 将接下来的45个字节作为数据推入栈
	OP_DATA_46     = 0x2e // 将接下来的46个字节作为数据推入栈
	OP_DATA_47     = 0x2f // 将接下来的47个字节作为数据推入栈
	OP_DATA_48     = 0x30 // 将接下来的48个字节作为数据推入栈
	OP_DATA_49     = 0x31 // 将接下来的49个字节作为数据推入栈
	OP_DATA_50     = 0x32 // 将接下来的50个字节作为数据推入栈
	OP_DATA_51     = 0x33 // 将接下来的51个字节作为数据推入栈
	OP_DATA_52     = 0x34 // 将接下来的52个字节作为数据推入栈
	OP_DATA_53     = 0x35 // 将接下来的53个字节作为数据推入栈
	OP_DATA_54     = 0x36 // 将接下来的54个字节作为数据推入栈
	OP_DATA_55     = 0x37 // 将接下来的55个字节作为数据推入栈
	OP_DATA_56     = 0x38 // 将接下来的56个字节作为数据推入栈
	OP_DATA_57     = 0x39 // 将接下来的57个字节作为数据推入栈
	OP_DATA_58     = 0x3a // 将接下来的58个字节作为数据推入栈
	OP_DATA_59     = 0x3b // 将接下来的59个字节作为数据推入栈
	OP_DATA_60     = 0x3c // 将接下来的60个字节作为数据推入栈
	OP_DATA_61     = 0x3d // 将接下来的61个字节作为数据推入栈
	OP_DATA_62     = 0x3e // 将接下来的62个字节作为数据推入栈
	OP_DATA_63     = 0x3f // 将接下来的63个字节作为数据推入栈
	OP_DATA_64     = 0x40 // 将接下来的64个字节作为数据推入栈
	OP_DATA_65     = 0x41 // 将接下来的65个字节作为数据推入栈
	OP_DATA_66     = 0x42 // 将接下来的66个字节作为数据推入栈
	OP_DATA_67     = 0x43 // 将接下来的67个字节作为数据推入栈
	OP_DATA_68     = 0x44 // 将接下来的68个字节作为数据推入栈
	OP_DATA_69     = 0x45 // 将接下来的69个字节作为数据推入栈
	OP_DATA_70     = 0x46 // 将接下来的70个字节作为数据推入栈
	OP_DATA_71     = 0x47 // 将接下来的71个字节作为数据推入栈
	OP_DATA_72     = 0x48 // 将接下来的72个字节作为数据推入栈
	OP_DATA_73     = 0x49 // 将接下来的73个字节作为数据推入栈
	OP_DATA_74     = 0x4a // 将接下来的74个字节作为数据推入栈
	OP_DATA_75     = 0x4b // 将接下来的75个字节作为数据推入栈
	OP_1           = 0x51 // 将数字1推入栈
	OP_PUSHDATA1   = 0x4c // 接下来的一个字节表示要推入栈的数据长度
	OP_PUSHDATA2   = 0x4d // 接下来的两个字节表示要推入栈的数据长度
	OP_PUSHDATA4   = 0x4e // 接下来的四个字节表示要推入栈的数据长度
	OP_1NEGATE     = 0x4f // 将数字-1推入栈
	OP_DUP         = 0x76 // 复制栈顶元素
	OP_HASH160     = 0xa9 // 对栈顶元素进行SHA-256然后RIPEMD-160哈希
	OP_CHECKSIG    = 0xac // 验证交易签名
	OP_EQUALVERIFY = 0x88 // 检查栈顶两个元素是否相等，如果相等则移除它们，否则失败
)

// Conditional 执行常数。
// 这些常量用于条件操作码的执行控制。
const (
	OpCondFalse = 0 // 条件为假
	OpCondTrue  = 1 // 条件为真
	OpCondSkip  = 2 // 跳过条件
)

// opcodeArray 保存有关所有可能的操作码的详细信息。
// 每个操作码都有其对应的处理函数、名称和长度信息。
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

	// 按位逻辑操作码
	OP_EQUALVERIFY: {OP_EQUALVERIFY, "OP_EQUALVERIFY", 1, opcodeEqualVerify},
}

// opcodeOnelineRepls 定义在进行单行反汇编时被替换的操作码名称。
// 这是为了使输出与参考实现保持一致。
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

// opcodePushData 是处理将原始数据推送到数据堆栈的通用函数。
// 参数:
//
//	op *opcode: 当前操作码
//	data []byte: 要推送的数据
//	vm *Engine: 虚拟机引擎
//
// 返回值:
//
//	error: 如果出错返回错误,否则返回nil
//
// 处理逻辑:
//  1. 将传入的数据直接推入虚拟机的数据栈
//  2. 返回nil表示操作成功
func opcodePushData(op *opcode, data []byte, vm *Engine) error {
	// 将数据推入虚拟机的数据栈
	vm.dstack.PushByteArray(data)
	// 返回nil表示操作成功
	return nil
}

// opcode1Negate 将-1推送到数据堆栈。
// 参数:
//
//	op *opcode: 当前操作码
//	data []byte: 未使用
//	vm *Engine: 虚拟机引擎
//
// 返回值:
//
//	error: 如果出错返回错误,否则返回nil
//
// 处理逻辑:
//  1. 创建一个表示-1的scriptNum
//  2. 将这个scriptNum推入虚拟机的数据栈
//  3. 返回nil表示操作成功
func opcode1Negate(op *opcode, data []byte, vm *Engine) error {
	// 将-1作为scriptNum推入虚拟机的数据栈
	vm.dstack.PushInt(scriptNum(-1))
	// 返回nil表示操作成功
	return nil
}

// opcodeDup 复制数据堆栈的顶部项目。
// 参数:
//
//	op *opcode: 当前操作码
//	data []byte: 未使用
//	vm *Engine: 虚拟机引擎
//
// 返回值:
//
//	error: 如果出错返回错误,否则返回nil
//
// 处理逻辑:
//  1. 调用虚拟机数据栈的DupN方法,复制栈顶元素
//  2. 返回操作结果
func opcodeDup(op *opcode, data []byte, vm *Engine) error {
	// 复制栈顶元素并返回结果
	return vm.dstack.DupN(1)
}

// opcodeHash160 计算栈顶元素的HASH160（先SHA256，再RIPEMD160）。
// 参数:
//
//	op *opcode: 当前操作码
//	data []byte: 未使用
//	vm *Engine: 虚拟机引擎
//
// 返回值:
//
//	error: 如果出错返回错误,否则返回nil
//
// 处理逻辑:
//  1. 从栈顶弹出一个字节数组
//  2. 对该字节数组进行SHA256哈希
//  3. 对SHA256的结果进行RIPEMD160哈希
//  4. 将最终的哈希结果推入栈
func opcodeHash160(op *opcode, data []byte, vm *Engine) error {
	// 从栈顶弹出一个字节数组
	buf, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}

	// 对弹出的数据进行SHA256哈希
	hash := sha256.Sum256(buf)
	// 对SHA256的结果进行RIPEMD160哈希,并将结果推入栈
	vm.dstack.PushByteArray(calcHash(hash[:], ripemd160.New()))
	return nil
}

// opcodeCheckSig 验证交易签名。
// 参数:
//
//	op *opcode: 当前操作码
//	data []byte: 交易数据
//	vm *Engine: 虚拟机引擎
//
// 返回值:
//
//	error: 如果出错返回错误,否则返回nil
//
// 处理逻辑:
//  1. 从栈顶弹出签名和公钥
//  2. 使用虚拟机的加密算法验证签名
//  3. 将验证结果(布尔值)推入栈
func opcodeCheckSig(op *opcode, data []byte, vm *Engine) error {
	// 从栈顶弹出签名
	signature, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}
	// 从栈顶弹出公钥
	pubKey, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}

	// 验证签名
	valid := vm.cryptoAlgo.VerifySignature(signature, data, pubKey)
	// 将验证结果推入栈
	vm.dstack.PushBool(valid)

	return nil
}

// opcodeEqualVerify 比较栈顶两个元素是否相等，并验证结果。
// 参数:
//
//	op *opcode: 当前操作码
//	data []byte: 未使用
//	vm *Engine: 虚拟机引擎
//
// 返回值:
//
//	error: 如果出错或验证失败返回错误,否则返回nil
//
// 处理逻辑:
//  1. 调用opcodeEqual比较栈顶两个元素
//  2. 如果相等,调用abstractVerify验证结果
func opcodeEqualVerify(op *opcode, data []byte, vm *Engine) error {
	// 比较栈顶两个元素是否相等
	err := opcodeEqual(vm)
	if err == nil {
		// 如果相等,验证结果
		err = abstractVerify(op, vm)
	}
	return err
}

// opcodeEqual 比较栈顶两个元素是否相等。
// 参数:
//
//	vm *Engine: 虚拟机引擎
//
// 返回值:
//
//	error: 如果出错返回错误,否则返回nil
//
// 处理逻辑:
//  1. 从栈顶弹出两个字节数组
//  2. 比较这两个字节数组是否相等
//  3. 将比较结果(布尔值)推入栈
func opcodeEqual(vm *Engine) error {
	// 从栈顶弹出第一个元素
	a, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}
	// 从栈顶弹出第二个元素
	b, err := vm.dstack.PopByteArray()
	if err != nil {
		return err
	}

	// 比较两个元素是否相等,并将结果推入栈
	vm.dstack.PushBool(bytes.Equal(a, b))
	return nil
}

// abstractVerify 验证栈顶元素是否为true。
// 参数:
//
//	op *opcode: 当前操作码
//	vm *Engine: 虚拟机引擎
//
// 返回值:
//
//	error: 如果验证失败返回错误,否则返回nil
//
// 处理逻辑:
//  1. 从栈顶弹出一个布尔值
//  2. 如果该布尔值为false,返回错误
//  3. 如果为true,返回nil
func abstractVerify(op *opcode, vm *Engine) error {
	// 从栈顶弹出一个布尔值
	verified, err := vm.dstack.PopBool()
	if err != nil {
		return err
	}

	// 如果弹出的值为false,返回错误
	if !verified {
		return fmt.Errorf("%s failed", op.name)
	}
	// 如果为true,返回nil
	return nil
}

// calcHash 计算给定数据的哈希值。
// 参数:
//
//	buf []byte: 要计算哈希的数据
//	hasher hash.Hash: 哈希算法实例
//
// 返回值:
//
//	[]byte: 计算得到的哈希值
//
// 处理逻辑:
//  1. 使用提供的哈希算法计算输入数据的哈希值
//  2. 返回计算得到的哈希值
func calcHash(buf []byte, hasher hash.Hash) []byte {
	// 将数据写入哈希算法
	hasher.Write(buf)
	// 计算并返回哈希值
	return hasher.Sum(nil)
}

// disasmOpcode 将操作码反汇编为人类可读的形式。
// 参数:
//
//	buf *strings.Builder: 用于构建反汇编字符串的 Builder
//	op *opcode: 要反汇编的操作码
//	data []byte: 与操作码相关的数据
//	compact bool: 是否使用紧凑格式
//
// 返回值:
//
//	无
//
// 处理逻辑:
//  1. 根据 compact 参数决定是否使用紧凑格式
//  2. 对于数据推送操作码，特殊处理其输出格式
//  3. 对于非数据推送操作码，直接输出操作码名称
//  4. 对于 PUSHDATA 操作码，添加数据长度信息
//  5. 将处理后的字符串写入 buf
func disasmOpcode(buf *strings.Builder, op *opcode, data []byte, compact bool) {
	// 获取操作码名称
	opcodeName := op.name

	// 如果使用紧凑格式，替换某些操作码名称
	if compact {
		if replName, ok := opcodeOnelineRepls[opcodeName]; ok {
			opcodeName = replName
		}

		// 根据操作码长度决定输出格式
		switch {
		case op.length == 1:
			// 对于长度为1的操作码，直接输出名称
			buf.WriteString(opcodeName)
		default:
			// 对于其他操作码，输出数据的十六进制表示
			buf.WriteString(hex.EncodeToString(data))
		}
		return
	}

	// 非紧凑格式处理
	buf.WriteString(opcodeName)

	switch op.length {
	case 1:
		// 对于长度为1的操作码，不需要额外处理
		return
	case -1:
		// 对于 PUSHDATA1，添加数据长度信息
		buf.WriteString(fmt.Sprintf(" 0x%02x", len(data)))
	case -2:
		// 对于 PUSHDATA2，添加数据长度信息
		buf.WriteString(fmt.Sprintf(" 0x%04x", len(data)))
	case -4:
		// 对于 PUSHDATA4，添加数据长度信息
		buf.WriteString(fmt.Sprintf(" 0x%08x", len(data)))
	}

	// 添加数据的十六进制表示
	buf.WriteString(fmt.Sprintf(" 0x%02x", data))
}
