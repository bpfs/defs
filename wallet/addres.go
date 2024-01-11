package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"math/big"

	"github.com/bpfs/defs/base58"
	"github.com/tyler-smith/go-bip32"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/ripemd160"
)

const version = byte(0x00)

// GetPubKeyHashFromPrivKey 从ECDSA私钥中提取公钥哈希。
// 参数 ownerPriv 是ECDSA私钥。
// 返回公钥哈希和可能发生的错误。
func GetPubKeyHashFromPrivKey(ownerPriv *ecdsa.PrivateKey) ([]byte, error) {
	pubKeyBytes := marshalPubKey(ownerPriv.PublicKey)
	return HashPubKey(pubKeyBytes), nil
}

// marshalPubKey 提取ECDSA公钥的字节表示。
// 参数 pubKey 是ECDSA公钥。
// 返回公钥的字节序列。
// 注意：这是将公钥转换为标准的非压缩形式。这种格式的前缀通常是 04，表示这是一个非压缩的公钥，后跟公钥的 X 和 Y 坐标
func marshalPubKey(pubKey ecdsa.PublicKey) []byte {
	return elliptic.Marshal(pubKey.Curve, pubKey.X, pubKey.Y)
}

// hashPubKey 返回给定钥的 SHA-256 哈希值的 RIPEMD160 哈希值。
// 参数 pubKeyBytes 是公钥的字节表示。
// 返回公钥的哈希值。
func HashPubKey(pubKey []byte) []byte {
	// 使用SHA256算法对公钥进行哈希
	h := sha256.Sum256(pubKey)
	return ripemd160h(h[:])
}

// ripemd160h 返回给定数据的 RIPEMD160 哈希值。
func ripemd160h(data []byte) []byte {
	// 创建一个RIPEMD160的哈希器
	h := ripemd160.New()
	// 将SHA256哈希的结果写入RIPEMD160哈希器
	h.Write(data)
	// 计算RIPEMD160哈希的结果
	return h.Sum(nil)
}

// GetPubKeyHash 从Base58编码的地址中提取公钥哈希。
// 参数 addressStr 是Base58编码的地址。
// 返回公钥哈希和可能发生的错误。
func GetPubKeyHash(addr string) ([]byte, error) {
	// 解码使用 CheckEncode 编码的字符串并验证校验和
	decoded, _, err := base58.CheckDecode(addr)
	if err != nil {
		return nil, err
	}

	// 地址的第一个字符是版本字节，公钥哈希是其余部分
	pubKeyHash := decoded[:]
	return pubKeyHash, nil
}

// ValidateAddress 检查地址是否有效
func ValidateAddress(address string) bool {
	_, _, err := base58.CheckDecode(address)
	return err == nil
}

// GetAddress 返回公钥钱包地址
func GetAddress(pubKey []byte) string {
	return encodeAddress(HashPubKey(pubKey), version)
}

// PubKeyHashGetAddress 返回公钥哈希对应的钱包地址
func PubKeyHashGetAddress(pubKeyHash []byte) string {
	return encodeAddress(pubKeyHash, version)
}

// encodeAddress 给定ripemd160哈希值和编码比特币网络和地址类型的netID，返回人类可读的支付地址。
// 它用于支付到公钥哈希 (P2PKH) 和支付到脚本哈希 (P2SH) 地址编码。
func encodeAddress(hash160 []byte, netID byte) string {
	// 网络和地址类别（即 P2PKH 与 P2SH）的格式为 1 个字节，RIPEMD160 哈希值的格式为 20 个字节，校验和为 4 个字节。
	return base58.CheckEncode(hash160[:ripemd160.Size], netID)
}

// GenerateECDSAKeyPair 从一个给定的种子（seed）生成椭圆曲线（Elliptic Curve）的密钥对.
// 接受种子（seed）、哈希函数类型（useSHA512）、盐（salt）、迭代次数（iterations）和密钥长度（keyLength）作为参数。
// 使用 PBKDF2 算法和指定的哈希函数从种子生成密钥。
// 然后，使用这个密钥和椭圆曲线算法生成 ECDSA 密钥对。
func GenerateECDSAKeyPair(password []byte, salt []byte, iterations, keyLength int, useSHA512 bool) (*ecdsa.PrivateKey, []byte, error) {
	curve := elliptic.P256() // 根据需要选择合适的曲线

	// 选择合适的哈希函数
	var hashFunc func() hash.Hash
	if useSHA512 {
		hashFunc = sha512.New
	} else {
		hashFunc = sha256.New
	}

	combined := append([]byte("BPFS"), salt...)

	// 使用 PBKDF2 生成强密钥
	key := pbkdf2.Key(password, combined, iterations, keyLength, hashFunc)

	// 生成主钱包
	masterKey, _ := bip32.NewMasterKey(key) //?????? 如果不使用启动host会报错

	// 生成私钥
	privateKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
		},
		D: new(big.Int).SetBytes(masterKey.Key),
	}

	// 计算公钥
	privateKey.PublicKey.X, privateKey.PublicKey.Y = curve.ScalarBaseMult(masterKey.Key)

	// 生成公钥
	// pubKey := append(privateKey.PublicKey.X.Bytes(), privateKey.PublicKey.Y.Bytes()...)
	// 注意：这是将公钥转换为标准的非压缩形式。这种格式的前缀通常是 04，表示这是一个非压缩的公钥，后跟公钥的 X 和 Y 坐标
	pubKey := elliptic.Marshal(privateKey.PublicKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y)

	return privateKey, pubKey, nil
}
