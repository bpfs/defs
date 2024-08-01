package wallets

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/bpfs/defs/debug"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

// Wallet 钱包主结构体，管理钱包数据和操作
type Wallet struct {
	sync.Mutex
	Mnemonics         sync.Map            // 助记词的存储，保证线程安全
	LocalRecords      map[string][16]byte // 本地记录，存储钱包地址和对应的密码MD5哈希
	CurrentlyLoggedIn *LoginInfo          // 当前登录的钱包信息
}

// LoginInfo 登录钱包信息结构体
type LoginInfo struct {
	Mnemonic   []byte            // 助记词
	PrivateKey *ecdh.PrivateKey // 私钥
	Password   [16]byte          // 密码的MD5哈希
}

// LocalWallets 本地钱包存储结构
type LocalWallets struct {
	Wallets map[string]WalletInfo // 存储钱包信息的映射，键为钱包地址
}

// WalletInfo 单个钱包的信息
type WalletInfo struct {
	Mnemonic   []byte      // 助记词
	PrivateKey []byte      // 私钥
	Password   [16]byte    // 密码的MD5哈希
	ImportTime time.Time   // 钱包导入时间
	LoginTimes []time.Time // 钱包登录时间记录
}

// InitWalletOutput 初始化钱包输出结构体
type InitWalletOutput struct {
	fx.Out
	Wallet *Wallet // 钱包
}

// InitWallet 初始化钱包，包括加载本地钱包数据和设置关闭时的保存逻辑
// 参数:
//   - lc (fx.Lifecycle): 应用程序的生命周期
//
// 返回值:
//   - InitWalletOutput: 包含初始化的钱包
//   - error: 如果初始化过程中发生错误
func InitWallet(lc fx.Lifecycle) (out InitWalletOutput, err error) {
	// 获取文件路径和AES加密密钥
	path, key, err := getFilePathAndKey()
	if err != nil {
		return out, err
	}

	// 初始化 Wallet 结构体，但不填充非 LocalRecords 的字段
	w := &Wallet{
		LocalRecords:      make(map[string][16]byte),
		CurrentlyLoggedIn: nil,
	}

	// 从本地文件加载钱包数据
	localWallets, err := loadFromFile(path, key)
	if err != nil {
		logrus.Errorf("[%s] 加载钱包数据失败: %v", debug.WhereAmI(), err)
		return out, err
	}

	// 填充 Wallet 的 LocalRecords 字段
	for address, info := range localWallets.Wallets {
		w.LocalRecords[address] = info.Password
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return saveToFile(path, localWallets, key) // 保存钱包数据到文件
		},
	})
	out.Wallet = w

	return out, nil
}

// GetSortedWalletAddresses 返回排序后的钱包地址数组
// 从 LocalRecords 提取所有钱包地址，进行排序，并返回排序后的地址数组
// 返回值:
//   - []string: 排序后的钱包地址数组
func (w *Wallet) GetSortedWalletAddresses() []string {
	w.Lock()         // 加锁以确保线程安全
	defer w.Unlock() // 在方法结束时解锁

	// 提取 LocalRecords 中的所有钱包地址
	var addresses []string
	for address := range w.LocalRecords {
		addresses = append(addresses, address)
	}

	// 对钱包地址进行排序
	sort.Strings(addresses)

	return addresses // 返回排序后的钱包地址数组
}

// DeleteWallet 删除本地钱包信息
// 输入钱包地址，如果存在则删除该钱包信息
// 参数:
//   - address (string): 钱包地址
//
// 返回值:
//   - error: 如果删除过程中发生错误
func (w *Wallet) DeleteWallet(address string) error {
	w.Lock()         // 加锁以确保线程安全
	defer w.Unlock() // 在方法结束时解锁

	// 从LocalRecords中删除地址
	_, exists := w.LocalRecords[address]
	if exists {
		delete(w.LocalRecords, address)
	}

	// 获取文件路径和AES加密密钥
	path, key, err := getFilePathAndKey()
	if err != nil {
		return err
	}

	// 从文件中加载LocalWallets
	localWallets, err := loadFromFile(path, key)
	if err != nil {
		logrus.Errorf("[%s] 加载本地钱包数据失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 从LocalWallets中删除钱包信息
	_, fileExists := localWallets.Wallets[address]
	if fileExists {
		delete(localWallets.Wallets, address)

		// 将更新后的LocalWallets写回文件
		if err := saveToFile(path, localWallets, key); err != nil {
			logrus.Errorf("[%s] 保存本地钱包数据失败: %v", debug.WhereAmI(), err)
			return err
		}
	} else if !exists {
		// 如果地址既不存在于内存也不存在于文件中，则记录警告
		logrus.Warnf("[%s] 钱包地址未找到: %s", debug.WhereAmI(), address)
		return fmt.Errorf("钱包地址未找到: %s", address)
	}

	return nil
}

// ClearAllWallets 清除本地所有钱包，包括已登录的信息
// 返回值:
//   - error: 如果清除过程中发生错误
func (w *Wallet) ClearAllWallets() error {
	w.Lock()
	defer w.Unlock()

	// 清空当前登录的钱包信息
	w.CurrentlyLoggedIn = nil

	// 获取文件路径
	path, _, err := getFilePathAndKey()
	if err != nil {
		return err
	}

	// 删除钱包文件
	err = os.Remove(path)
	if err != nil {
		logrus.Errorf("[%s] 删除钱包文件失败: %v", debug.WhereAmI(), err)
		return err
	}

	// 清空LocalRecords
	w.LocalRecords = make(map[string][16]byte)

	return nil
}

//////////////////////////////////////////////////////////////////////////////////////////

/**

// GetWalletDetails 返回给定地址的钱包详细信息
func (w *Wallet) GetWalletDetails(address string) (*WalletInfo, error) {
	address = strings.TrimSpace(address)

	wallet, ok := w.Wallets[address]
	if !ok {
		return nil, fmt.Errorf("未找到钱包地址：%s", address)
	}
	// TODO:余额查询，助记词二进制
	return &wallet, nil
}

// RemoveWallet 从内存池中删除钱包并保存到文件
func (w *Wallet) RemoveWallet(address string) error {
	w.Lock()
	defer w.Unlock()

	address = strings.TrimSpace(address)

	delete(w.Wallets, address)
	return w.saveToFile()
}

// SaveToFile 将钱包数据保存到本地文件
// 在创建文件之前，检查文件路径的目录是否存在，如果不存在，则创建它。
// 在写入文件前，可以先写入到临时文件，然后再将其重命名为最终文件，以防止在写入过程中发生错误导致数据丢失。
func (w *Wallet) saveToFile() error {
	// 确保文件目录存在
	if err := os.MkdirAll(filepath.Dir(w.FilePath), 0755); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 写入临时文件
	tempFilePath := w.FilePath + ".tmp"
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	encoder := gob.NewEncoder(tempFile)
	if err := encoder.Encode(w.Wallets); err != nil {
		tempFile.Close()
		os.Remove(tempFilePath) // 清理临时文件

		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	tempFile.Close()
	// 重命名为最终文件
	return os.Rename(tempFilePath, w.FilePath)
}

// addWalletToPool 添加基于助记词的钱包到内存池
func (w *Wallet) AddWalletToPool(mnemonic, password string) error {
	mnemonic = strings.TrimSpace(mnemonic)
	password = strings.TrimSpace(password)

	if !bip39.IsMnemonicValid(mnemonic) {
		return fmt.Errorf("助记词验证失败")
	}
	if password == "" {
		return fmt.Errorf("密码不可为空")
	}

	// 创建并返回一个钱包
	walletInfo, err := newWallet([]byte(mnemonic), []byte(password))
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 向内存池中添加钱包
	return w.AddWallet(walletInfo)
}

// VerifyMnemonicAndPassword 验证助记词和密码，返回钱包地址的后6位
func (w *Wallet) VerifyMnemonicAndPassword(mnemonic, password string) (string, error) {
	mnemonic = strings.TrimSpace(mnemonic)
	password = strings.TrimSpace(password)

	if password == "" {
		return "", fmt.Errorf("密码不可为空")
	}
	if !bip39.IsMnemonicValid(mnemonic) {
		return "", fmt.Errorf("助记词验证失败")
	}

	// 创建并返回一个钱包
	walletInfo, err := newWallet([]byte(mnemonic), []byte(password))
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return "", err
	}

	// 返回公钥钱包地址
	address := GetAddress(walletInfo.PublicKey)

	last6 := address[len(address)-6:]

	// 确保地址的后6位未被使用，有则从映射中删除条目
	delete(w.MnemonicAddressMap, last6)

	// 存储到内存池映射中
	w.MnemonicAddressMap[last6] = []string{mnemonic, password}
	return last6, nil
}

// AddMnemonic 向内存池添加助记词
func (w *Wallet) AddMnemonic(hash string, mnemonic interface{}) {
	hash = strings.TrimSpace(hash)
	w.Mnemonics.Store(hash, mnemonic)
}

// GetMnemonic 从内存池获取助记词
func (w *Wallet) GetMnemonic(hash string) (interface{}, bool) {
	hash = strings.TrimSpace(hash)
	return w.Mnemonics.Load(hash)
}

// RemoveMnemonic 从内存池中删除助记词
func (w *Wallet) RemoveMnemonic(hash string) {
	hash = strings.TrimSpace(hash)
	w.Mnemonics.Delete(hash)
}

// AddWallet 向内存池添加钱包并保存到文件
func (w *Wallet) AddWallet(walletInfo *WalletInfo) error {
	w.Lock()
	defer w.Unlock()

	if len(walletInfo.Mnemonic) == 0 || len(walletInfo.PrivateKey) == 0 || len(walletInfo.PublicKey) == 0 || len(walletInfo.Password) == 0 {
		logrus.Errorf("[%s]从内存池获取钱包时失败", debug.WhereAmI())
		return fmt.Errorf("向内存池添加钱包时失败")
	}

	// 向内存池添加钱包信息
	w.Mnemonic = walletInfo.Mnemonic     // 助记词
	w.PrivateKey = walletInfo.PrivateKey // 私钥
	w.PublicKey = walletInfo.PublicKey   // 公钥
	w.Password = walletInfo.Password     // 密码

	// 返回公钥钱包地址
	address := wallet.GetAddress(walletInfo.PublicKey)
	w.Wallets[address] = *walletInfo

	return w.saveToFile()
}

// GetWallet 从内存池获取钱包
func (w *Wallet) GetWallet(address, password string) error {
	w.Lock()
	defer w.Unlock()

	address = strings.TrimSpace(address)
	password = strings.TrimSpace(password)

	walletInfo, ok := w.Wallets[address]
	if !ok {
		return fmt.Errorf("钱包地址不存在")
	}

	if len(walletInfo.Mnemonic) == 0 || len(walletInfo.PrivateKey) == 0 || len(walletInfo.PublicKey) == 0 || len(walletInfo.Password) == 0 {
		logrus.Errorf("[%s]从内存池获取钱包时失败", debug.WhereAmI())
		// 删除键值对
		delete(w.Wallets, address)
		return fmt.Errorf("钱包记录不完整")
	}

	// [16]byte 是一个固定大小的字节数组类型，因此它可以直接使用 != 进行比较
	if walletInfo.Password != md5.Sum([]byte(password)) {
		return fmt.Errorf("钱包密码不正确")
	}

	// 向内存池添加钱包信息
	w.Mnemonic = walletInfo.Mnemonic     // 助记词
	w.PrivateKey = walletInfo.PrivateKey // 私钥
	w.PublicKey = walletInfo.PublicKey   // 公钥
	w.Password = walletInfo.Password     // 密码

	return nil
}

// GetAllWalletAddresses 返回内存池中所有钱包的地址
func (w *Wallet) GetAllWalletAddresses() []string {
	w.Lock()
	defer w.Unlock()
	var addresses []string
	for address := range w.Wallets {
		addresses = append(addresses, address)
	}
	return addresses
}

// GetMnemonicByWalletAddress 根据钱包地址获取助记词
func (w *Wallet) GetMnemonicByWalletAddress(address string) (string, error) {
	address = strings.TrimSpace(address)

	wallet, ok := w.Wallets[address]
	if !ok {
		return "", fmt.Errorf("未找到钱包地址：%s", address)
	}

	return string(wallet.Mnemonic), nil
}

// getAddress 返回钱包地址
// func (w *Wallet) getAddress() string {
// 	if len(w.Mnemonic) == 0 {
// 		return ""
// 	}
// 	return wallet.GetAddress(w.PublicKey)
// }

// getMnemonic 返回钱包助记词
func (w *Wallet) getMnemonic() string {
	return string(w.Mnemonic)
}

// getPrivateKey 返回钱包私钥
func (w *Wallet) getPrivateKey() (*ecdh.PrivateKey, error) {
	return x509.ParseECPrivateKey(w.PrivateKey)
}

// getPublicKey 返回钱包公钥哈希
func (w *Wallet) getPubKeyHash() []byte {
	return wallet.HashPubKey(w.PublicKey)
}

// newWallet 创建并返回一个钱包
func newWallet(mnemonic, password []byte) (*WalletInfo, error) {
	privateKey, publicKey, err := wallet.GenerateECDSAKeyPair(mnemonic, password, 4096, elliptic.P256().Params().BitSize/8, false)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}
	privBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return nil, err
	}

	walletInfo := &WalletInfo{
		Mnemonic:   mnemonic,          // 助记词
		PrivateKey: privBytes,         // 私钥
		PublicKey:  publicKey,         // 公钥
		Password:   md5.Sum(password), // 密码
	}

	// 创建并初始化 Wallet 结构体
	// wallet := &Wallet{
	// 	WalletInfo: walletInfo, // 使用上面创建的 WalletInfo 实例
	// }

	return walletInfo, nil
}

// resetWalletInfo 将WalletInfo 字段重新初始化为零
func (w *Wallet) resetWalletInfo() {
	w.WalletInfo = WalletInfo{
		Mnemonic:   nil,
		PrivateKey: nil,
		PublicKey:  nil,
		Password:   [16]byte{},
	}
}



*/
