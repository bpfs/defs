package script

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	logging "github.com/dep2p/log"
	"golang.org/x/crypto/ripemd160"
	"log"
	"math/big"
	"strings"
)

var logger = logging.Logger("script")

// 定义脚本相关的常量
const (
	// MaxScriptElementSize 定义可推入堆栈的最大字节数
	MaxScriptElementSize = 520
)

// IsPayToPubKeyHash 检查脚本是否为标准的支付公钥哈希(P2PKH)格式
// 参数:
//   - script: 要检查的脚本字节切片
//
// 返回:
//   - bool: 如果是P2PKH格式则返回true,否则返回false
func IsPayToPubKeyHash(script []byte) bool {
	return isPubKeyHashScript(script) // 调用内部函数检查是否为P2PKH脚本
}

// DisasmString 将脚本反汇编为一行字符串
// 参数:
//   - script: 要反汇编的脚本字节切片
//
// 返回:
//   - string: 反汇编后的字符串
//   - error: 解析过程中的错误,如果没有错误则为nil
//
// 注意: 此函数仅适用于版本0的脚本
func DisasmString(script []byte) (string, error) {
	const scriptVersion = 0 // 定义脚本版本

	var disbuf strings.Builder                              // 创建字符串构建器
	tokenizer := MakeScriptTokenizer(scriptVersion, script) // 创建脚本分词器
	if tokenizer.Next() {                                   // 如果有下一个token
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), true) // 反汇编第一个操作码
	}
	for tokenizer.Next() { // 遍历剩余的token
		disbuf.WriteByte(' ')                                       // 写入空格分隔符
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), true) // 反汇编操作码
	}
	if tokenizer.Err() != nil { // 如果分词器出错
		if tokenizer.ByteIndex() != 0 { // 如果不是在开始位置出错
			disbuf.WriteByte(' ') // 写入空格
		}
		disbuf.WriteString("[error]") // 写入错误标记
	}
	return disbuf.String(), tokenizer.Err() // 返回反汇编结果和可能的错误
}

// VerifyScriptPubKeyHash 验证脚本中的公钥哈希是否与给定的PubKeyHash匹配
// 参数:
//   - script: 要验证的脚本字节切片
//   - pubKeyHash: 要匹配的公钥哈希
//
// 返回:
//   - bool: 如果匹配则返回true,否则返回false
func VerifyScriptPubKeyHash(script, pubKeyHash []byte) bool {
	if len(script) < 25 { // 检查脚本长度是否足够
		return false // 脚本长度不足，返回false
	}

	if script[0] != OP_DUP || script[1] != OP_HASH160 { // 检查OP_DUP和OP_HASH160
		return false // 脚本不是有效的P2PKH脚本，返回false
	}

	if script[2] != 0x14 { // 检查pubKeyHash长度（通常为20字节）
		return false // 无效的公钥哈希长度，返回false
	}

	extractedPubKeyHash := script[3:23] // 提取脚本中的公钥哈希

	return bytes.Equal(extractedPubKeyHash, pubKeyHash) // 比较提取的公钥哈希与提供的公钥哈希
}

// ExtractPubKeyFromP2PKScriptToECDSA 从P2PK脚本中提取ECDSA公钥
// 参数:
//   - p2pkScript: P2PK脚本的字节切片
//
// 返回:
//   - *ecdsa.PublicKey: 提取的ECDSA公钥
//   - error: 提取过程中的错误,如果没有错误则为nil
func ExtractPubKeyFromP2PKScriptToECDSA(p2pkScript []byte) (*ecdsa.PublicKey, error) {
	if len(p2pkScript) < 2 || p2pkScript[len(p2pkScript)-1] != 0xAC { // 检查OP_CHECKSIG
		return nil, fmt.Errorf("无效的P2PK脚本") // 返回错误
	}
	pubKeyBytes := p2pkScript[1 : len(p2pkScript)-1] // 移除OP_CHECKSIG

	x, y := elliptic.Unmarshal(elliptic.P256(), pubKeyBytes) // 解析公钥坐标
	if x == nil || y == nil {
		return nil, fmt.Errorf("无法解析公钥") // 返回错误
	}

	return &ecdsa.PublicKey{ // 返回ECDSA公钥
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}, nil
}

// ExtractPubKeyHashFromP2PKScript 从 P2PK 脚本中提取公钥哈希
// 参数:
//   - p2pkScript: P2PK 公钥脚本的字节切片
//
// 返回:
//   - []byte: 提取的公钥哈希
//   - error: 提取过程中的错误,如果没有错误则为nil
//
// 处理逻辑:
//  1. 验证脚本长度和结构
//  2. 提取公钥字节
//  3. 对公钥进行SHA256哈希
//  4. 对SHA256哈希结果进行RIPEMD160哈希
func ExtractPubKeyHashFromP2PKScript(p2pkScript []byte) ([]byte, error) {
	// 调用 DisassembleScript 来反汇编脚本
	disassembledScript := DisassembleScript(p2pkScript)
	logger.Infof("反汇编脚本:\t%s\n", disassembledScript)

	if len(p2pkScript) < 2 || p2pkScript[len(p2pkScript)-1] != OP_CHECKSIG { // 验证脚本结构
		return nil, fmt.Errorf("无效的 P2PK 脚本") // 返回错误
	}
	pubKeyBytes := p2pkScript[1 : len(p2pkScript)-1] // 移除 OP_CHECKSIG

	h := sha256.Sum256(pubKeyBytes) // 使用SHA256算法对公钥进行哈希

	hasher := ripemd160.New()    // 创建RIPEMD160哈希器
	_, err := hasher.Write(h[:]) // 写入SHA256哈希结果
	if err != nil {
		log.Panic(err) // 如果出错，记录并抛出panic
	}
	return hasher.Sum(nil), nil // 返回RIPEMD160哈希结果
}

// ExtractPubKeyFromP2PKScriptToRSA 从P2PK脚本中提取RSA公钥
// 参数:
//   - p2pkScript: P2PK 公钥脚本的字节切片
//
// 返回:
//   - *rsa.PublicKey: 提取的RSA公钥
//   - error: 提取过程中的错误,如果没有错误则为nil
//
// 处理逻辑:
//  1. 验证脚本长度和结构
//  2. 提取公钥字节
//  3. 解析公钥字节为PKIX格式
//  4. 将解析结果转换为RSA公钥
func ExtractPubKeyFromP2PKScriptToRSA(p2pkScript []byte) (*rsa.PublicKey, error) {
	// 检查脚本长度是否至少为2字节,并且最后一个字节是否为OP_CHECKSIG (0xAC)
	if len(p2pkScript) < 2 || p2pkScript[len(p2pkScript)-1] != 0xAC {
		return nil, errors.New("无效的P2PK脚本")
	}

	pubKeyBytes := p2pkScript[3 : len(p2pkScript)-1] // 提取公钥字节

	pub, err := x509.ParsePKIXPublicKey(pubKeyBytes) // 解析PKIX格式的公钥
	if err != nil {
		return nil, err // 返回解析错误
	}

	rsaPub, ok := pub.(*rsa.PublicKey) // 类型断言为RSA公钥
	if !ok {
		return nil, errors.New("公钥类型不是RSA") // 返回类型错误
	}

	return rsaPub, nil // 返回RSA公钥
}

// ExtractPubKeyHashFromScript 从P2PKH脚本中提取公钥哈希
// 参数:
//   - script: P2PKH 脚本的字节切片
//
// 返回:
//   - []byte: 提取的公钥哈希
//   - error: 提取过程中的错误,如果没有错误则为nil
//
// 处理逻辑:
//  1. 验证脚本长度
//  2. 从脚本中提取公钥哈希部分(通常是第3到第22个字节)
func ExtractPubKeyHashFromScript(script []byte) ([]byte, error) {
	if len(script) < 25 { // 检查脚本长度
		return nil, fmt.Errorf("script too short") // 返回错误
	}
	pubKeyHash := script[3:23] // 提取公钥哈希部分
	return pubKeyHash, nil     // 返回公钥哈希
}

// DisassembleScript 反汇编脚本并以易读的格式返回
// 参数:
//   - script: 要反汇编的脚本字节切片
//
// 返回:
//   - string: 反汇编后的脚本字符串,操作码和数据以空格分隔
//
// 处理逻辑:
//  1. 遍历脚本字节
//  2. 识别每个字节代表的操作码或数据
//  3. 将操作码转换为可读字符串
//  4. 对于数据推送操作,提取并编码数据
//  5. 将所有解析结果拼接成一个字符串
func DisassembleScript(script []byte) string {
	var disassembled []string // 初始化存储反汇编结果的切片
	var i int                 // 初始化索引变量
	for i < len(script) {     // 遍历脚本字节
		op := script[i] // 获取当前字节作为操作码
		i++             // 移动到下一个字节

		switch { // 根据操作码进行处理
		case op >= OP_DATA_1 && op <= OP_DATA_75: // 数据推送操作码
			dataLength := int(op)           // 计算数据长度
			if i+dataLength > len(script) { // 检查是否有足够的字节
				disassembled = append(disassembled, "ERROR: Malformed data push") // 添加错误信息
				break
			}
			data := script[i : i+dataLength]                              // 提取数据
			disassembled = append(disassembled, hex.EncodeToString(data)) // 将数据转换为十六进制字符串并添加到结果中
			i += dataLength                                               // 移动索引到数据之后

		case op == OP_DUP: // OP_DUP操作码
			disassembled = append(disassembled, "OP_DUP") // 添加OP_DUP到结果中

		case op == OP_HASH160: // OP_HASH160操作码
			disassembled = append(disassembled, "OP_HASH160") // 添加OP_HASH160到结果中

		case op == OP_EQUALVERIFY: // OP_EQUALVERIFY操作码
			disassembled = append(disassembled, "OP_EQUALVERIFY") // 添加OP_EQUALVERIFY到结果中

		case op == OP_CHECKSIG: // OP_CHECKSIG操作码
			disassembled = append(disassembled, "OP_CHECKSIG") // 添加OP_CHECKSIG到结果中

		default: // 未知的操作码
			disassembled = append(disassembled, fmt.Sprintf("UNKNOWN_OPCODE_%x", op)) // 添加未知操作码到结果中
		}
	}

	return strings.Join(disassembled, " ") // 将切片转换为单个字符串，用空格分隔
}

// CompressPubKey 获取压缩格式的公钥
// 参数:
//   - pubKey: 要压缩的ECDSA公钥
//
// 返回:
//   - []byte: 压缩后的公钥字节切片
//
// 处理逻辑:
//  1. 获取公钥的X坐标
//  2. 根据Y坐标的奇偶性确定前缀
//  3. 将前缀和X坐标组合成压缩公钥
func CompressPubKey(pubKey *ecdsa.PublicKey) []byte {
	xBytes := pubKey.X.Bytes() // 获取公钥的X坐标

	var prefix byte           // 定义前缀变量
	if pubKey.Y.Bit(0) == 0 { // 判断Y坐标的奇偶性
		prefix = 0x02 // Y是偶数，前缀为0x02
	} else {
		prefix = 0x03 // Y是奇数，前缀为0x03
	}

	return append([]byte{prefix}, xBytes...) // 返回压缩公钥（前缀 + X坐标）
}

// DecompressPubKey 从压缩公钥解压得到完整的公钥
// 参数:
//   - curve: 使用的椭圆曲线
//   - compressedPubKey: 压缩格式的公钥字节切片
//
// 返回:
//   - *ecdsa.PublicKey: 解压后的ECDSA公钥
//   - error: 解压过程中的错误,如果没有错误则为nil
func DecompressPubKey(curve elliptic.Curve, compressedPubKey []byte) (*ecdsa.PublicKey, error) {
	if len(compressedPubKey) != 33 { // 检查压缩公钥长度
		return nil, errors.New("invalid compressed public key length") // 返回错误
	}

	prefix := compressedPubKey[0]         // 获取前缀
	if prefix != 0x02 && prefix != 0x03 { // 检查前缀是否有效
		return nil, errors.New("invalid compressed public key prefix") // 返回错误
	}

	x := new(big.Int).SetBytes(compressedPubKey[1:]) // 设置X坐标
	y, err := decompressY(curve, x, prefix == 0x03)  // 计算Y坐标
	if err != nil {
		return nil, err // 返回错误
	}

	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil // 返回完整的公钥
}

// decompressY 根据X坐标和奇偶性计算Y坐标
// 参数:
//   - curve: 使用的椭圆曲线
//   - x: X坐
//   - odd: Y坐标的奇偶性
//
// 返回:
//   - *big.Int: 计算得到的Y坐标
//   - error: 计算过程中的错误,如果没有错误则为nil
func decompressY(curve elliptic.Curve, x *big.Int, odd bool) (*big.Int, error) {
	params := curve.Params() // 获取曲线参数

	// 使用正确的椭圆曲线方程: y^2 = x^3 - 3x + b
	xCubed := new(big.Int).Exp(x, big.NewInt(3), params.P) // 计算x^3
	threeX := new(big.Int).Mul(x, big.NewInt(3))           // 计算3x
	threeX.Mod(threeX, params.P)                           // 对3x取模
	xCubed.Sub(xCubed, threeX)                             // x^3 - 3x
	xCubed.Add(xCubed, params.B)                           // (x^3 - 3x) + b
	xCubed.Mod(xCubed, params.P)                           // 对结果取模

	y := new(big.Int).ModSqrt(xCubed, params.P) // 计算平方根
	if y == nil {
		return nil, errors.New("error computing Y coordinate") // 返回错误
	}

	if odd != isOdd(y) { // 调整Y的符号
		y.Sub(params.P, y) // 如果奇偶性不匹配，取补数
	}

	return y, nil // 返回Y坐标
}

// isOdd 判断一个大整数是否为奇数
// 参数:
//   - a: 要判断的大整数
//
// 返回:
//   - bool: 如果是奇数则返回true,否则返回false
func isOdd(a *big.Int) bool {
	return a.Bit(0) == 1 // 检查最低位是否为1
}
