package uploads

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"fmt"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/shamir"
	"github.com/bpfs/defs/wallets"

	"github.com/sirupsen/logrus"
)

// FileSecurity 封装了文件的安全和权限相关的信息
type FileSecurity struct {
	Secret        []byte            // 文件加密密钥
	EncryptionKey [][]byte          // 文件加密密钥，用于在上传过程中保证文件数据的安全
	PrivateKey    *ecdh.PrivateKey // 文件签名密钥
	P2PKHScript   []byte            // P2PKH 脚本，用于区块链场景中验证文件所有者的身份
	P2PKScript    []byte            // P2PK 脚本，用于区块链场景中进行文件验签操作
}

// NewFileSecurity 创建并初始化一个新的FileSecurity实例，封装了文件的安全和权限相关的信息
func NewFileSecurity(privKey *ecdh.PrivateKey, file afero.File, scheme *shamir.ShamirScheme, secret []byte) (*FileSecurity, error) {
	// 生成份额
	shares, err := scheme.GenerateShares(secret)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	if len(shares) == 0 {
		return nil, fmt.Errorf("生成共享密钥时失败")
	}

	// 生成并设置输入的公钥哈希。
	publicKey := privKey.PublicKey
	publicKeyBytes := elliptic.Marshal(publicKey.Curve, publicKey.X, publicKey.Y)

	// 构建P2PK脚本
	p2pk, err := script.NewScriptBuilder().
		AddData(publicKeyBytes).   // 直接添加公钥
		AddOp(script.OP_CHECKSIG). // 添加检查签名操作
		Script()
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	logrus.Printf("P2PK 十六进制脚本: %x", hex.EncodeToString(p2pk))

	// 通过私钥生成公钥哈希
	pubKeyHash, ok := wallets.PrivateKeyToPublicKeyHash(privKey) // 从ECDSA私钥中提取公钥哈希
	if !ok {
		return nil, fmt.Errorf("通过私钥生成公钥哈希时失败")
	}
	logrus.Printf("pubKeyHash 公钥哈希: %x", hex.EncodeToString(pubKeyHash))

	// 构建P2PKH脚本
	p2pkh, err := script.NewScriptBuilder().
		AddOp(script.OP_DUP).AddOp(script.OP_HASH160).
		AddData(pubKeyHash).
		AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG).
		Script()
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	logrus.Printf("P2PKH 十六进制脚本: %x", hex.EncodeToString(p2pkh))

	return &FileSecurity{
		Secret:        secret,  // 文件加密密钥
		EncryptionKey: shares,  // 文件加密密钥
		PrivateKey:    privKey, // 文件签名密钥
		P2PKHScript:   p2pkh,   // P2PKH 脚本
		P2PKScript:    p2pk,    // P2PK 脚本
	}, nil
}
