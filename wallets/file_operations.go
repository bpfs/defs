package wallets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/macs"
	"github.com/bpfs/defs/paths"
	"github.com/sirupsen/logrus"
)

// loadFromFile 从本地文件加载钱包数据，使用AES-GCM进行加密数据的安全读取
// 参数:
//   - filePath (string): 文件路径
//   - key ([]byte): AES加密密钥
//
// 返回值:
//   - *LocalWallets: 读取到的钱包数据
//   - error: 如果读取过程中发生错误
func loadFromFile(filePath string, key []byte) (*LocalWallets, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &LocalWallets{Wallets: make(map[string]WalletInfo)}, nil // 文件不存在视为初次运行，返回空的 LocalWallets 结构
		}
		return nil, err
	}
	defer file.Close()

	block, err := aes.NewCipher(key) // 创建AES加密实例
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block) // 创建GCM模式的AES实例
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize()) // 创建随机数
	if _, err := io.ReadFull(file, nonce); err != nil {
		return nil, err
	}

	encryptedData, err := io.ReadAll(file) // 读取加密数据
	if err != nil {
		return nil, err
	}

	data, err := gcm.Open(nil, nonce, encryptedData, nil) // 解密数据
	if err != nil {
		return nil, err
	}

	localWallets := &LocalWallets{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(localWallets); err != nil {
		return nil, err
	}

	return localWallets, nil // 返回 LocalWallets 结构
}

// saveToFile 将钱包数据保存到本地文件，加密内容
// 使用临时文件来确保写入过程中不会破坏原有文件内容
// 参数:
//   - filePath (string): 文件路径
//   - localWallets (*LocalWallets): 要保存的钱包数据
//   - key ([]byte): AES加密密钥
//
// 返回值:
//   - error: 如果保存过程中发生错误
func saveToFile(filePath string, localWallets *LocalWallets, key []byte) error {
	// 创建AES加密实例
	block, err := aes.NewCipher(key)
	if err != nil {
		logrus.Errorf("[%s] 创建AES加密实例失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 创建GCM模式的AES实例
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		logrus.Errorf("[%s] 创建GCM模式失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 创建随机数
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		logrus.Errorf("[%s] 生成随机数失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 编码本地钱包数据
	dataBuffer := new(bytes.Buffer)
	if err := gob.NewEncoder(dataBuffer).Encode(localWallets); err != nil {
		logrus.Errorf("[%s] 编码本地钱包数据失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 加密数据
	encryptedData := gcm.Seal(nonce, nonce, dataBuffer.Bytes(), nil)

	// 确保文件目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		logrus.Errorf("[%s] 创建目录失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 创建临时文件路径
	tempFilePath := filePath + ".tmp"

	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		logrus.Errorf("[%s] 创建临时文件失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 将加密数据写入临时文件
	if _, err := tempFile.Write(encryptedData); err != nil {
		tempFile.Close()
		os.Remove(tempFilePath) // 清理临时文件
		logrus.Errorf("[%s] 写入临时文件失败: %v", debug.WhereAmI(), err)
		return err
	}

	tempFile.Close()

	// 删除目标文件（如果存在），避免重命名冲突
	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			logrus.Errorf("[%s] 删除原有文件失败: %v", debug.WhereAmI(), err)
			return err
		}
	}

	// 重命名临时文件为最终文件
	if err := os.Rename(tempFilePath, filePath); err != nil {
		os.Remove(tempFilePath) // 清理临时文件
		logrus.Errorf("[%s] 重命名文件失败: %v", debug.WhereAmI(), err)
		return err
	}

	return nil
}

// getFilePathAndKey 获取文件路径和AES加密密钥
// 返回文件路径和AES加密密钥
// 返回值:
//   - string: 文件路径
//   - []byte: AES加密密钥
//   - error: 如果获取过程中发生错误
func getFilePathAndKey() (string, []byte, error) {
	// 获取MAC地址
	macAddress, err := macs.GetPrimaryMACAddress()
	if err != nil {
		logrus.Errorf("[%s] 获取MAC地址失败: %v", debug.WhereAmI(), err)
		return "", nil, err
	}

	// 生成AES加密密钥
	key := md5.Sum([]byte(macAddress))   // 计算MAC地址的MD5哈希值
	dir := md5.Sum(key[:])               // 对MAC的MD5值再次计算MD5，用作存储目录
	folderName := fmt.Sprintf("%x", dir) // 将MD5值转换为十六进制字符串
	path := filepath.Join(paths.GetRootPath(), folderName, ".wallets")

	return path, key[:], nil
}
