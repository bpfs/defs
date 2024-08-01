package wallets

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/bpfs/defs/debug"
	"github.com/cosmos/go-bip39"
	"github.com/sirupsen/logrus"
)

// GenerateNewMnemonic 生成新助记词
// 返回值:
//   - []string: 生成的助记词数组
//   - error: 如果生成过程中发生错误
func (w *Wallet) GenerateNewMnemonic() ([]string, error) {
	// 创建随机熵字节
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		logrus.Errorf("[%s] 创建随机熵字节失败: %v", debug.WhereAmI(), err)
		return nil, fmt.Errorf("创建随机熵字节失败: %v", err)
	}

	// 生成助记词
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		logrus.Errorf("[%s] 生成助记词失败: %v", debug.WhereAmI(), err)
		return nil, fmt.Errorf("生成助记词失败: %v", err)
	}

	// 拆分助记词
	words := strings.Split(mnemonic, " ")

	// 生成助记词哈希
	hash := md5.Sum([]byte(words[0] + words[len(words)-1]))
	hashStr := hex.EncodeToString(hash[:])

	// 添加助记词到内存池
	w.AddMnemonic(hashStr, mnemonic)

	return words, nil
}

// AddMnemonic 向内存池添加助记词
// 参数:
//   - hash (string): 助记词哈希
//   - mnemonic (string): 助记词
func (w *Wallet) AddMnemonic(hash string, mnemonic string) {
	hash = strings.TrimSpace(hash)
	w.Mnemonics.Store(hash, mnemonic)
}

// GetMnemonic 从内存池获取助记词，并删除键值
// 参数:
//   - hash (string): 助记词哈希
//
// 返回值:
//   - string: 助记词
//   - bool: 是否成功找到助记词
func (w *Wallet) GetMnemonic(hash string) (string, bool) {
	hash = strings.TrimSpace(hash)
	// 删除某个键的值，并返回之前的值（如果有）
	mnemonic, ok := w.Mnemonics.LoadAndDelete(hash)
	if !ok {
		return "", false
	}

	// 确保返回类型为string
	mnemonicStr, ok := mnemonic.(string)
	if !ok {
		logrus.Errorf("[%s] 从内存池获取助记词时类型时失败: %v", debug.WhereAmI(), mnemonic)
		return "", false
	}

	return mnemonicStr, true
}

// RemoveMnemonic 从内存池删除助记词
// 参数:
//   - hash (string): 助记词哈希
func (w *Wallet) RemoveMnemonic(hash string) {
	hash = strings.TrimSpace(hash)
	w.Mnemonics.Delete(hash)
}
