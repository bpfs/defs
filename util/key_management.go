package util

import (
	"crypto/ecdsa"
	"crypto/x509"
	"errors"

	"github.com/bpfs/dep2p/utils"
	"github.com/sirupsen/logrus"
)

// MarshalPrivateKey 将ECDSA私钥序列化为字节表示
// 参数:
//   - privateKey (*ecdsa.PrivateKey): 输入的ECDSA私钥
//
// 返回值:
//   - []byte: 私钥的字节序列
//   - error: 失败时的错误信息
func MarshalPrivateKey(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	//	if privateKey == nil || privateKey.D == nil {
	if privateKey == nil {
		err := errors.New("无效的私钥")
		logrus.Errorf("[%s] 无效的私钥: %v", utils.WhereAmI(), err)
		return nil, err
	}
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		logrus.Errorf("[%s]: %v", utils.WhereAmI(), err)
		return nil, err
	}
	//return privateKey.D.Bytes(), nil
	return privateKeyBytes, nil
}

// UnmarshalPrivateKey 将字节序列反序列化为ECDSA私钥
// 参数:
//   - privKeyBytes ([]byte): 私钥的字节表示
//
// 返回值:
//   - *ecdsa.PrivateKey: 反序列化后的ECDSA私钥
//   - error: 失败时的错误信息
func UnmarshalPrivateKey(privKeyBytes []byte) (*ecdsa.PrivateKey, error) {
	if len(privKeyBytes) == 0 {
		err := errors.New("无效的私钥字节")
		logrus.Errorf("[%s] 无效的私钥字节: %v", utils.WhereAmI(), err)
		return nil, err
	}

	// // PEM解码
	// privBlock, _ := pem.Decode(privKeyBytes)
	// if privBlock == nil {
	// 	err := errors.New("私钥PEM解码失败")
	// 	logrus.Errorf("[%s] 私钥PEM解码失败: %v", utils.WhereAmI(), err)
	// 	return nil, err
	// }

	// 解析ASN.1 DER格式的私钥
	//privateKey, err := x509.ParseECPrivateKey(privBlock.Bytes)
	privateKey, err := x509.ParseECPrivateKey(privKeyBytes)
	if err != nil {
		logrus.Errorf("[%s] 解析私钥失败: %v", utils.WhereAmI(), err)
		return nil, err
	}
	return privateKey, nil
}
