package script

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"errors"
	"math/big"

	"golang.org/x/crypto/ripemd160"
)

// base58Alphabet 定义了Base58编码所使用的字符集。
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// marshalPubKey 提取ECDSA公钥的字节表示。
// 参数 pubKey 是ECDSA公钥。
// 返回公钥的字节序列。
func marshalPubKey(pubKey ecdsa.PublicKey) []byte {
	return elliptic.Marshal(pubKey.Curve, pubKey.X, pubKey.Y)
}

// hashPubKey 对公钥执行SHA256和RIPEMD-160哈希。
// 参数 pubKeyBytes 是公钥的字节表示。
// 返回公钥的哈希值。
func hashPubKey(pubKeyBytes []byte) []byte {
	sha256Hash := sha256.Sum256(pubKeyBytes)
	ripemd160Hasher := ripemd160.New()
	ripemd160Hasher.Write(sha256Hash[:])
	return ripemd160Hasher.Sum(nil)
}

// DecodeBase58Check 对Base58Check编码的字符串进行解码。
// 参数 input 是Base58Check编码的字符串。
// 返回解码后的字节序列和可能发生的错误。
func DecodeBase58Check(input string) ([]byte, error) {
	// Base58解码
	decoded := base58Decode(input)
	if len(decoded) < 4 {
		return nil, errors.New("Base58Check字符串太短")
	}

	// 检查并删除校验和
	checksum := decoded[len(decoded)-4:]
	payload := decoded[:len(decoded)-4]

	// 计算校验和
	hash := doubleSha256(payload)
	if !bytes.Equal(hash[:4], checksum) {
		return nil, errors.New("Base58Check校验和不匹配")
	}

	return payload, nil
}

// base58Decode 将Base58编码的字符串解码为字节切片。
// 参数 input 是Base58编码的字符串。
// 返回解码后的字节序列。
func base58Decode(input string) []byte {
	// 初始化字符集映射
	alphabetIndex := map[rune]*big.Int{}
	for i, char := range base58Alphabet {
		alphabetIndex[char] = big.NewInt(int64(i))
	}

	// 解码逻辑
	result := big.NewInt(0)
	multiplier := big.NewInt(1)
	for i := len(input) - 1; i >= 0; i-- {
		value, ok := alphabetIndex[rune(input[i])]
		if !ok {
			return []byte{}
		}
		result.Add(result, new(big.Int).Mul(multiplier, value))
		multiplier.Mul(multiplier, big.NewInt(58))
	}

	decoded := result.Bytes()

	// 处理前导零
	for _, char := range input {
		if char != '1' {
			break
		}
		decoded = append([]byte{0}, decoded...)
	}

	return decoded
}

// doubleSha256 对数据执行两次SHA256哈希。
// 参数 data 是待哈希的数据。
// 返回哈希值。
func doubleSha256(data []byte) []byte {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second[:]
}

// GetPubKeyHash 从Base58编码的地址中提取公钥哈希。
// 参数 addressStr 是Base58编码的地址。
// 返回公钥哈希和可能发生的错误。
func GetPubKeyHash(addressStr string) ([]byte, error) {
	decoded, err := DecodeBase58Check(addressStr)
	if err != nil {
		return nil, err
	}

	// 地址的第一个字符是版本字节，公钥哈希是其余部分
	pubKeyHash := decoded[1:]
	return pubKeyHash, nil
}

// GetPubKeyHashFromPrivKey 从ECDSA私钥中提取公钥哈希。
// 参数 ownerPriv 是ECDSA私钥。
// 返回公钥哈希和可能发生的错误。
func GetPubKeyHashFromPrivKey(ownerPriv *ecdsa.PrivateKey) ([]byte, error) {
	pubKeyBytes := marshalPubKey(ownerPriv.PublicKey)
	pubKeyHash := hashPubKey(pubKeyBytes)
	return pubKeyHash, nil
}

// GetAddressFromPrivKey 从ECDSA私钥生成地址。
// 参数 ownerPriv 是ECDSA私钥。
// 返回地址和可能发生的错误。
func GetAddressFromPrivKey(ownerPriv *ecdsa.PrivateKey) (string, error) {
	pubKeyBytes := marshalPubKey(ownerPriv.PublicKey)
	pubKeyHash := hashPubKey(pubKeyBytes)

	versionedPayload := append([]byte{0x00}, pubKeyHash...)
	address := base58CheckEncode(versionedPayload)
	return address, nil
}

// base58CheckEncode 对数据执行Base58Check编码。
// 参数 input 是待编码的数据。
// 返回Base58Check编码的字符串。
func base58CheckEncode(input []byte) string {
	checksum := doubleSha256(input)
	fullPayload := append(input, checksum[:4]...)
	return base58Encode(fullPayload)
}

// base58Encode 将字节切片编码为Base58字符串。
// 参数 input 是待编码的字节切片。
// 返回Base58编码的字符串。
func base58Encode(input []byte) string {
	x := big.NewInt(0).SetBytes(input)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := &big.Int{}
	var result []byte

	for x.Cmp(zero) != 0 {
		x.DivMod(x, base, mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}

	// 处理前导零
	for _, b := range input {
		if b != 0 {
			break
		}
		result = append(result, base58Alphabet[0])
	}

	// 反转结果
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}
