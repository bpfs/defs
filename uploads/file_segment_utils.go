package uploads

import (
	"crypto/ecdsa"
	"crypto/md5"
	"os"

	"github.com/bpfs/defs/v2/pb"
	sign "github.com/bpfs/defs/v2/sign/ecdsa"
)

// cleanupTempFiles 清理临时文件
// 参数:
//   - files: 临时文件数组
func cleanupTempFiles(files []*os.File) {
	for _, f := range files {
		if f != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}
}

// generateSignature 使用私钥对数据进行签名
// 参数:
//   - privateKey: ECDSA私钥，用于生成签名
//   - data: 需要签名的数据结构
//
// 返回值:
//   - []byte: 生成的签名数据
//   - error: 如果签名失败，返回错误信息
func generateSignature(privateKey *ecdsa.PrivateKey, data *pb.SignatureData) ([]byte, error) {
	// 序列化数据
	dataBytes, err := data.Marshal()
	if err != nil {
		logger.Errorf("序列化SignatureData失败: err=%v", err)
		return nil, err
	}

	// 计算哈希
	hash := md5.Sum(dataBytes)
	merged := hash[:]

	// 生成签名
	signature, err := sign.SignData(privateKey, merged)
	if err != nil {
		logger.Errorf("签名数据失败: err=%v", err)
		return nil, err
	}

	return signature, nil
}
