package uploads

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"

	"github.com/bpfs/defs/v2/files"
	"github.com/bpfs/defs/v2/pb"
	"github.com/bpfs/defs/v2/script"
)

// NewFileSecurity 创建并初始化一个新的FileSecurity实例，封装了文件的安全和权限相关的信息
// 参数:
//   - fileID: string 文件ID
//   - privKey: *ecdsa.PrivateKey 私钥
//   - secret: []byte 需要共享的秘密
//
// 返回:
//   - *pb.FileSecurity: 包含文件安全信息的结构体
//   - error: 错误信息
func NewFileSecurity(fileID string, privKey *ecdsa.PrivateKey, secret []byte) (*pb.FileSecurity, error) {
	// 生成密钥分片
	shares, err := files.GenerateKeyShares(privKey, fileID)
	if err != nil {
		logger.Errorf("生成密钥分片失败: %v, fileID: %s", err, fileID)
		return nil, err
	}

	// 从私钥获取公钥
	publicKey := privKey.PublicKey
	// 序列化公钥为字节数组
	publicKeyBytes := elliptic.Marshal(publicKey.Curve, publicKey.X, publicKey.Y)

	// 构建P2PK脚本
	p2pk, err := script.NewScriptBuilder().
		AddData(publicKeyBytes).   // 直接添加公钥
		AddOp(script.OP_CHECKSIG). // 添加检查签名操作
		Script()
	if err != nil {
		logger.Errorf("构建P2PK脚本失败: %v, fileID: %s", err, fileID)
		return nil, err
	}
	// logger.Infof("P2PK 十六进制脚本: %s", hex.EncodeToString(p2pk))

	// 通过私钥生成公钥哈希
	pubKeyHash, ok := files.PrivateKeyToPublicKeyHash(privKey)
	if !ok {
		logger.Errorf("通过私钥生成公钥哈希失败, fileID: %s", fileID)
		return nil, fmt.Errorf("通过私钥生成公钥哈希失败")
	}
	// logger.Infof("pubKeyHash 公钥哈希: %s", hex.EncodeToString(pubKeyHash))

	// 构建P2PKH脚本
	p2pkh, err := script.NewScriptBuilder().
		AddOp(script.OP_DUP).AddOp(script.OP_HASH160).          // 复制栈顶元素并计算哈希
		AddData(pubKeyHash).                                    // 添加公钥哈希
		AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG). // 验证相等并检查签名
		Script()
	if err != nil {
		logger.Errorf("构建P2PKH脚本失败: %v, fileID: %s", err, fileID)
		return nil, err
	}
	// logger.Infof("P2PKH 十六进制脚本: %s", hex.EncodeToString(p2pkh))

	// 序列化私钥
	ownerPriv, err := files.MarshalPrivateKey(privKey)
	if err != nil {
		logger.Errorf("序列化私钥失败: %v, fileID: %s", err, fileID)
		return nil, err
	}

	// 返回FileSecurity结构体
	return &pb.FileSecurity{
		Secret:        secret,    // 文件加密密钥
		EncryptionKey: shares,    // 文件加密密钥的共享份额
		OwnerPriv:     ownerPriv, // 序列化后的所有者的私钥
		P2PkhScript:   p2pkh,     // P2PKH 脚本
		P2PkScript:    p2pk,      // P2PK 脚本
	}, nil
}
