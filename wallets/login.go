package wallets

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bpfs/defs/debug"
	"github.com/cosmos/go-bip39"
	"github.com/sirupsen/logrus"
)

// GetLoggedInMnemonic 返回当前登录钱包的助记词
// 返回值:
//   - []string: 当前登录钱包的助记词，如果没有登录钱包则返回nil
func (w *Wallet) GetLoggedInMnemonic() ([]string, bool) {
	w.Lock()
	defer w.Unlock()

	if w.CurrentlyLoggedIn == nil {
		return nil, false
	}
	mnemonic := string(w.CurrentlyLoggedIn.Mnemonic)
	return strings.Split(mnemonic, " "), true
}

// GetLoggedInAddress 返回当前登录钱包的地址
// 返回值:
//   - string: 当前登录钱包的地址，如果没有登录钱包则返回空字符串
func (w *Wallet) GetLoggedInAddress() (string, bool) {
	w.Lock()
	defer w.Unlock()

	if w.CurrentlyLoggedIn == nil {
		return "", false
	}

	// 通过私钥生成钱包地址
	address, ok := PrivateKeyToAddress(w.CurrentlyLoggedIn.PrivateKey)
	if !ok {
		return "", false
	}
	return address, true
}

// GetLoggedInPublicKey 返回当前登录钱包的公钥
// 返回值:
//   - []byte: 当前登录钱包的公钥字节，如果没有登录钱包则返回nil
//   - bool: 返回操作是否成功
func (w *Wallet) GetLoggedInPublicKey() ([]byte, bool) {
	w.Lock()
	defer w.Unlock()

	// 检查是否有已登录的信息
	if w.CurrentlyLoggedIn == nil {
		logrus.Errorf("[%s] 当前没有已登录的钱包信息", debug.WhereAmI())
		return nil, false
	}

	// 提取公钥
	publicKey := ExtractPublicKey(w.CurrentlyLoggedIn.PrivateKey)
	return publicKey, true
}

// GetLoggedInPublicKeyHash 返回当前登录钱包的公钥哈希
// 返回值:
//   - []byte: 当前登录钱包的公钥哈希，如果没有登录钱包则返回nil
//   - bool: 返回操作是否成功
func (w *Wallet) GetLoggedInPublicKeyHash() ([]byte, bool) {
	w.Lock()
	defer w.Unlock()

	// 检查是否有已登录的信息
	if w.CurrentlyLoggedIn == nil {
		logrus.Errorf("[%s] 当前没有已登录的钱包信息", debug.WhereAmI())
		return nil, false
	}

	// 提取公钥
	publicKey := ExtractPublicKey(w.CurrentlyLoggedIn.PrivateKey)

	// 计算公钥哈希
	publicKeyHash := HashPublicKey(publicKey)
	return publicKeyHash, true
}

// IsLoggedInValid 检查当前登录的钱包信息是否完整和有效
// 返回值:
//   - bool: true 如果登录信息有效，否则 false
func (w *Wallet) IsLoggedInValid() bool {
	w.Lock()
	defer w.Unlock()

	// 检查是否有已登录的信息
	if w.CurrentlyLoggedIn == nil {
		logrus.Errorf("[%s] 当前没有已登录的钱包信息", debug.WhereAmI())
		return false
	}

	// 检查助记词是否存在且非空
	if len(w.CurrentlyLoggedIn.Mnemonic) == 0 {
		logrus.Errorf("[%s] 登录信息中的助记词为空", debug.WhereAmI())
		return false
	}

	// 检查私钥是否存在且非空
	if w.CurrentlyLoggedIn.PrivateKey == nil {
		logrus.Errorf("[%s] 登录信息中的私钥为空", debug.WhereAmI())
		return false
	}

	// 检查密码哈希是否存在且长度正确
	if len(w.CurrentlyLoggedIn.Password) != 16 {
		logrus.Errorf("[%s] 登录信息中的密码哈希长度不正确", debug.WhereAmI())
		return false
	}

	return true
}

// CheckLoggedInPassword 校验当前登录钱包的密码是否正确
// 参数:
//   - password (string): 输入的密码
//
// 返回值:
//   - bool: true表示密码正确，否则false
func (w *Wallet) CheckLoggedInPassword(password string) bool {
	w.Lock()
	defer w.Unlock()

	if w.CurrentlyLoggedIn == nil {
		return false
	}

	// 计算输入密码的MD5哈希
	inputPasswordHash := md5.Sum([]byte(password))

	// 比较输入密码的哈希值和当前登录钱包的密码哈希
	return w.CurrentlyLoggedIn.Password == inputPasswordHash
}

// ImportWalletWithMnemonic 使用助记词导入钱包的公共子方法
// 参数:
//   - mnemonic (string): 助记词
//   - password (string): 用户提供的密码
//
// 返回值:
//   - error: 处理过程中发生的任何错误
//
// 用途说明:
//   - 使用助记词生成密钥对，并将钱包导入系统。
//   - 最终将钱包设为当前登录状态。
func (w *Wallet) ImportWalletWithMnemonic(hash, password string) error {
	// 从内存池获取助记词，并删除键值
	mnemonic, ok := w.GetMnemonic(hash)
	if !ok {
		return fmt.Errorf("从内存池获取助记词时失败")
	}

	// 尝试验证提供的助记符是否有效
	if !bip39.IsMnemonicValid(mnemonic) {
		return fmt.Errorf("助记符验证失败")
	}

	// 使用助记词生成密钥对
	salt := []byte(mnemonic)
	iterations := 4096
	keyLength := elliptic.P256().Params().BitSize / 8
	useSHA512 := false

	privateKey, _, err := GenerateECDSAKeyPair(salt, []byte(password), iterations, keyLength, useSHA512)
	if err != nil {
		logrus.Errorf("[%s] 生成密钥对失败: %v", debug.WhereAmI(), err)
		return fmt.Errorf("生成密钥对失败: %v", err)
	}

	// 将助记词、私钥和密码导入钱包
	if err := w.ImportWallet([]byte(mnemonic), privateKey, md5.Sum([]byte(password))); err != nil {
		logrus.Errorf("[%s] 导入钱包失败: %v", debug.WhereAmI(), err)
		return fmt.Errorf("导入钱包失败: %v", err)
	}

	// 通过私钥生成钱包地址
	address, ok := PrivateKeyToAddress(privateKey)
	if !ok {
		return fmt.Errorf("私钥生成钱包地址时失败")
	}

	// 选择钱包地址和输入密码登录钱包
	return w.LoginWallet(address, password)
}

// LoginWallet 选择钱包地址和输入密码登录钱包
// 参数:
//   - address (string): 钱包地址
//   - password (string): 输入的密码
//
// 返回值:
//   - error: 如果登录失败
func (w *Wallet) LoginWallet(address, password string) error {
	w.Lock()
	defer w.Unlock()

	// 清空当前登录的钱包信息
	w.CurrentlyLoggedIn = nil

	// 检查地址是否在本地记录中
	passwordHash, exists := w.LocalRecords[address]
	if !exists {
		return fmt.Errorf("钱包地址未找到: %s", address)
	}

	// 校验输入密码是否正确
	inputPasswordHash := md5.Sum([]byte(password))
	if passwordHash != inputPasswordHash {
		return fmt.Errorf("密码错误")
	}

	// 从文件中加载LocalWallets
	path, key, err := getFilePathAndKey()
	if err != nil {
		return err
	}
	// 从本地文件加载钱包数据，使用AES-GCM进行加密数据的安全读取
	localWallets, err := loadFromFile(path, key)
	if err != nil {
		return err
	}

	// 查找并设置当前登录的钱包信息
	walletInfo, exists := localWallets.Wallets[address]
	if !exists {
		return fmt.Errorf("钱包地址未找到: %s", address)
	}

	// 校验钱包信息中的核心数据是否存在
	if len(walletInfo.Mnemonic) == 0 || len(walletInfo.PrivateKey) == 0 || len(walletInfo.Password) != 16 {
		return fmt.Errorf("钱包信息不完整: %s", address)
	}

	// 将字节序列反序列化为ECDSA私钥
	privateKey, err := UnmarshalPrivateKey(walletInfo.PrivateKey)
	if err != nil {
		return err
	}
	w.CurrentlyLoggedIn = &LoginInfo{
		Mnemonic:   walletInfo.Mnemonic,
		PrivateKey: privateKey,
		Password:   walletInfo.Password,
	}

	// 记录登录时间
	walletInfo.LoginTimes = append(walletInfo.LoginTimes, time.Now())

	// 更新LocalWallets并保存到文件
	localWallets.Wallets[address] = walletInfo
	if err := saveToFile(path, localWallets, key); err != nil {
		return err
	}

	return nil
}

// LogoutWallet 退出当前登录的钱包
// 返回值:
//   - error: 如果退出过程中发生错误
func (w *Wallet) LogoutWallet() error {
	w.Lock()
	defer w.Unlock()

	if w.CurrentlyLoggedIn == nil {
		return fmt.Errorf("没有已登录的钱包")
	}

	// 清空CurrentlyLoggedIn
	w.CurrentlyLoggedIn = nil

	return nil
}

// ImportWallet 导入钱包
// 参数:
//   - mnemonic ([]byte): 助记词
//   - privateKey (*ecdsa.PrivateKey): 私钥
//   - passwordHash ([16]byte): 密码的MD5哈希
//
// 返回值:
//   - error: 如果导入失败
func (w *Wallet) ImportWallet(mnemonic []byte, privateKey *ecdsa.PrivateKey, passwordHash [16]byte) error {
	w.Lock()
	defer w.Unlock()

	// 参数检查
	if len(mnemonic) == 0 {
		err := errors.New("助记词不能为空")
		logrus.Errorf("[%s] 助记词不能为空: %v", debug.WhereAmI(), err)
		return err
	}
	if privateKey == nil {
		err := errors.New("私钥不能为空")
		logrus.Errorf("[%s] 私钥不能为空: %v", debug.WhereAmI(), err)
		return err
	}
	if len(passwordHash) != 16 {
		err := errors.New("密码哈希长度无效")
		logrus.Errorf("[%s] 密码哈希长度无效: %v", debug.WhereAmI(), err)
		return err
	}

	// 通过私钥生成钱包地址
	address, ok := PrivateKeyToAddress(privateKey)
	if !ok {
		return fmt.Errorf("私钥生成钱包地址时失败")
	}

	// 获取文件路径和AES加密密钥
	path, key, err := getFilePathAndKey()
	if err != nil {
		return err
	}

	// 从文件中加载LocalWallets
	localWallets, err := loadFromFile(path, key)
	if err != nil {
		return err
	}

	// 删除现有的钱包信息
	delete(localWallets.Wallets, address)

	// 导入钱包信息
	privKeyBytes, err := MarshalPrivateKey(privateKey)
	if err != nil {
		return err
	}
	walletInfo := WalletInfo{
		Mnemonic:   mnemonic,
		PrivateKey: privKeyBytes, // 序列化私钥
		Password:   passwordHash,
		ImportTime: time.Now(),
	}
	localWallets.Wallets[address] = walletInfo
	w.LocalRecords[address] = passwordHash

	// 保存更新后的LocalWallets到文件
	if err := saveToFile(path, localWallets, key); err != nil {
		return err
	}

	return nil
}
