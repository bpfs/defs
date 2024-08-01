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
	"log"
	"math/big"
	"strings"

	"golang.org/x/crypto/ripemd160"
)

// 这些是为各个脚本中的最大值指定的常量。
const (
	// MaxOpsPerScript       = 201 // 最大非推送操作数。
	// MaxPubKeysPerMultiSig = 20  // 多重签名不能有比这更多的签名。
	MaxScriptElementSize = 520 // 可推入堆栈的最大字节数。
)

// 如果脚本采用标准支付公钥哈希 (P2PKH) 格式，则 IsPayToPubKeyHash 返回 true，否则返回 false。
func IsPayToPubKeyHash(script []byte) bool {
	return isPubKeyHashScript(script)
}

// DisasmString 将反汇编脚本格式化为一行打印。 当脚本解析失败时，返回的字符串将包含失败发生点之前的反汇编脚本，并附加字符串'[error]'。 此外，如果调用者想要有关失败的更多信息，则会返回脚本解析失败的原因。
//
// 注意：该函数仅对0版本脚本有效。 由于该函数不接受脚本版本，因此其他脚本版本的结果未定义。
func DisasmString(script []byte) (string, error) {
	const scriptVersion = 0

	var disbuf strings.Builder
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	if tokenizer.Next() {
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), true)
	}
	for tokenizer.Next() {
		disbuf.WriteByte(' ')
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), true)
	}
	if tokenizer.Err() != nil {
		if tokenizer.ByteIndex() != 0 {
			disbuf.WriteByte(' ')
		}
		disbuf.WriteString("[error]")
	}
	return disbuf.String(), tokenizer.Err()
}

// VerifyScriptPubKeyHash 验证脚本中的公钥哈希是否与给定的PubKeyHash匹配
func VerifyScriptPubKeyHash(script, pubKeyHash []byte) bool {
	// P2PKH脚本的典型结构: OP_DUP OP_HASH160 <pubKeyHash> OP_EQUALVERIFY OP_CHECKSIG
	if len(script) < 25 {
		return false // 脚本长度不足
	}

	// 检查OP_DUP和OP_HASH160
	if script[0] != OP_DUP || script[1] != OP_HASH160 {
		return false // 脚本不是有效的P2PKH脚本
	}

	// 检查pubKeyHash长度（通常为20字节）
	if script[2] != 0x14 {
		return false // 无效的公钥哈希长度
	}

	// 提取脚本中的公钥哈希
	extractedPubKeyHash := script[3:23]

	// 比较提取的公钥哈希与提供的公钥哈希
	return bytes.Equal(extractedPubKeyHash, pubKeyHash)
}

// ExtractPubKeyFromP2PKScriptToECDSA 从P2PK脚本中提取ECDSA公钥
func ExtractPubKeyFromP2PKScriptToECDSA(p2pkScript []byte) (*ecdsa.PublicKey, error) {
	if len(p2pkScript) < 2 || p2pkScript[len(p2pkScript)-1] != 0xAC { // 检查OP_CHECKSIG
		return nil, fmt.Errorf("无效的P2PK脚本")
	}
	pubKeyBytes := p2pkScript[1 : len(p2pkScript)-1] // 移除OP_CHECKSIG

	// logrus.Printf("提取的公钥字节: %x\n", pubKeyBytes) // 打印公钥字节

	x, y := elliptic.Unmarshal(elliptic.P256(), pubKeyBytes)
	if x == nil || y == nil {
		return nil, fmt.Errorf("无法解析公钥")
	}

	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}, nil
}

// ExtractPubKeyHashFromP2PKScript 从 P2PK 脚本中提取公钥哈希。
// 参数 p2pkScript 是 P2PK 公钥脚本的字节表示。
// 返回公钥的哈希值。
func ExtractPubKeyHashFromP2PKScript(p2pkScript []byte) ([]byte, error) {
	if len(p2pkScript) < 2 || p2pkScript[len(p2pkScript)-1] != OP_CHECKSIG {
		return nil, fmt.Errorf("无效的 P2PK 脚本")
	}
	pubKeyBytes := p2pkScript[1 : len(p2pkScript)-1] // 移除 OP_CHECKSIG

	// 使用SHA256算法对公钥进行哈希
	h := sha256.Sum256(pubKeyBytes)

	// 使用RIPEMD160算法对SHA256哈希值进行哈希
	hasher := ripemd160.New()
	_, err := hasher.Write(h[:])
	if err != nil {
		log.Panic(err)
	}
	return hasher.Sum(nil), nil
}

// ExtractPubKeyFromP2PKScriptToRSA 从P2PK脚本中提取RSA公钥
func ExtractPubKeyFromP2PKScriptToRSA(p2pkScript []byte) (*rsa.PublicKey, error) {
	if len(p2pkScript) < 2 || p2pkScript[len(p2pkScript)-1] != 0xAC { // 检查OP_CHECKSIG
		return nil, errors.New("无效的P2PK脚本")
	}

	// 移除脚本中的OP_CHECKSIG，获取公钥字节
	pubKeyBytes := p2pkScript[3 : len(p2pkScript)-1]

	// 将字节解码为公钥
	pub, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		return nil, err
	}

	// 断言解析出的公钥类型为 *rsa.PublicKey
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("公钥类型不是RSA")
	}

	return rsaPub, nil
}

// ExtractPubKeyHashFromScript 从P2PKH提取公钥哈希
func ExtractPubKeyHashFromScript(script []byte) ([]byte, error) {
	// 解析脚本，这里假设脚本格式正确
	if len(script) < 25 { // 简单检查，因为P2PKH脚本应该正好是25字节
		return nil, fmt.Errorf("script too short")
	}
	// 按照P2PKH脚本的结构，公钥哈希应该从第三个元素开始，长度为20
	pubKeyHash := script[3:23] // 索引从0开始，因此第三个元素的索引是2，但是这里OP_HASH160之后
	return pubKeyHash, nil
}

// 反汇编脚本并以易读的格式返回
func DisassembleScript(script []byte) string {
	var disassembled []string
	var i int
	for i < len(script) {
		op := script[i]
		i++

		switch {
		case op >= OP_DATA_1 && op <= OP_DATA_75: // Data push opcodes
			dataLength := int(op)
			if i+dataLength > len(script) {
				disassembled = append(disassembled, "ERROR: Malformed data push")
				break
			}
			data := script[i : i+dataLength]
			disassembled = append(disassembled, hex.EncodeToString(data))
			i += dataLength

		case op == OP_DUP: // OP_DUP
			disassembled = append(disassembled, "OP_DUP")

		case op == OP_HASH160: // OP_HASH160
			disassembled = append(disassembled, "OP_HASH160")

		case op == OP_EQUALVERIFY: // OP_EQUALVERIFY
			disassembled = append(disassembled, "OP_EQUALVERIFY")

		case op == OP_CHECKSIG: // OP_CHECKSIG
			disassembled = append(disassembled, "OP_CHECKSIG")

		default:
			disassembled = append(disassembled, fmt.Sprintf("UNKNOWN_OPCODE_%x", op)) // 未知的操作码
		}
	}

	// 将切片转换为单个字符串，用空格分隔
	return strings.Join(disassembled, " ")
}

// 获取压缩公钥
func CompressPubKey(pubKey *ecdsa.PublicKey) []byte {
	// 获取公钥的X坐标
	xBytes := pubKey.X.Bytes()

	// 前缀：0x02表示Y是偶数，0x03表示Y是奇数
	var prefix byte
	if pubKey.Y.Bit(0) == 0 {
		prefix = 0x02
	} else {
		prefix = 0x03
	}

	// 压缩公钥为前缀 + X坐标
	return append([]byte{prefix}, xBytes...)
}

// 从压缩公钥解压
func DecompressPubKey(curve elliptic.Curve, compressedPubKey []byte) (*ecdsa.PublicKey, error) {
	if len(compressedPubKey) != 33 {
		return nil, errors.New("invalid compressed public key length")
	}

	prefix := compressedPubKey[0]
	if prefix != 0x02 && prefix != 0x03 {
		return nil, errors.New("invalid compressed public key prefix")
	}

	x := new(big.Int).SetBytes(compressedPubKey[1:])
	y, err := decompressY(curve, x, prefix == 0x03)
	if err != nil {
		return nil, err
	}

	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

func decompressY(curve elliptic.Curve, x *big.Int, odd bool) (*big.Int, error) {
	params := curve.Params()

	// 使用正确的椭圆曲线方程: y^2 = x^3 - 3x + b
	xCubed := new(big.Int).Exp(x, big.NewInt(3), params.P)
	threeX := new(big.Int).Mul(x, big.NewInt(3))
	threeX.Mod(threeX, params.P)
	xCubed.Sub(xCubed, threeX)
	xCubed.Add(xCubed, params.B)
	xCubed.Mod(xCubed, params.P)

	// 计算平方根
	y := new(big.Int).ModSqrt(xCubed, params.P)
	if y == nil {
		return nil, errors.New("error computing Y coordinate")
	}

	// 调整Y的符号
	if odd != isOdd(y) {
		y.Sub(params.P, y)
	}

	return y, nil
}

func isOdd(a *big.Int) bool {
	return a.Bit(0) == 1
}
