package shamir

import (
	"crypto/sha256"
	"io"
	"math/big"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"
)

// 定义或生成prime
var prime, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)

// 测试生成份额和恢复秘密的基本功能
func TestGenerateSharesAndRecoverSecret(t *testing.T) {
	// 定义测试用的秘密和参数
	secret := []byte("Hello, Shamir Secret Sharing!")
	n := 5 // 总份额数
	k := 3 // 需要恢复秘密的最小份额数

	// 使用GenerateShares生成份额
	shares, err := GenerateStandardShares(secret, n, k)
	if err != nil {
		t.Errorf("生成份额失败: %v", err)
	}

	// 验证生成的份额数量是否正确
	if len(shares) != n {
		t.Errorf("期望生成 %d 份额，但是生成了 %d 份额", n, len(shares))
	}

	// 打印所有生成的份额
	logger.Infof("生成的所有密钥分片:\n")
	for i, share := range shares {
		logger.Infof("份额 #%d: (x=%s, y=%s)\n", i+1, share[0].Text(10), share[1].Text(10))
	}

	// 随机选择k个份额来恢复秘密
	selectedShares := make([][2]*big.Int, k)
	for i, index := range randomSample(n, k) {
		selectedShares[i] = shares[index]
	}

	// 打印用于恢复的份额
	logger.Infof("用于恢复的密钥分片:\n")
	for i, share := range selectedShares {
		logger.Infof("份额 #%d: (x=%s, y=%s)\n", i+1, share[0].Text(10), share[1].Text(10))
	}

	// 使用RecoverSecret恢复秘密
	recoveredSecretBytes, err := RecoverSecret(selectedShares, prime)
	if err != nil {
		t.Errorf("恢复秘密失败: %v", err)
	}

	// 比较原始秘密和恢复后的秘密是否一致
	if !reflect.DeepEqual(secret, recoveredSecretBytes) {
		t.Errorf("原始秘密与恢复后的秘密不匹配\n原始: %s\n恢复: %s", secret, recoveredSecretBytes)
	} else {
		logger.Infof("\n成功恢复秘密\n原始: %s\n恢复: %s", secret, recoveredSecretBytes)
	}
}

// randomSample 提供一个简单的方法来随机选择k个不同的索引，用于从n个份额中选择k个

// randomSample 从n个元素中随机选择k个不同的索引
func randomSample(n, k int) []int {
	rand.Seed(time.Now().UnixNano()) // 初始化随机数生成器

	// 创建一个长度为n的切片，用于生成n个连续的整数
	nums := make([]int, n)
	for i := range nums {
		nums[i] = i
	}

	// 随机选择k个不同的索引
	selected := make([]int, k)
	for i := 0; i < k; i++ {
		// 生成一个随机索引
		r := i + rand.Intn(n-i)
		// 将选中的元素（索引）保存到selected中
		selected[i] = nums[r]
		// 将选中的元素换到当前位置，以避免被重复选择
		nums[r] = nums[i]
	}

	return selected
}

func TestGenerateSharesWithFixedShare(t *testing.T) {
	// 假设的文件路径
	filePath := "/Users/qinglong/go/src/chaincodes/BPFS/DeFS/defs/shamir/shamir.go"

	// 计算文件的哈希值作为固定份额的y值
	fixedShareY, err := CalculateFileHash(filePath)
	if err != nil {
		t.Fatalf("计算文件哈希失败: %v", err)
	}
	// 假设固定份额的x值为1（通常需要确保这个x值在生成其他份额时不会被重复使用）
	fixedShareX := big.NewInt(1)

	// 定义测试用的秘密和参数
	secret := []byte("Hello, Shamir Secret Sharing!")
	n := 5 // 总份额数
	k := 2 // 需要恢复秘密的最小份额数

	// 使用GenerateSharesWithFixedShare生成份额，包括固定份额
	shares, err := GenerateSharesWithFixedShare(secret, n, k, fixedShareX, fixedShareY, prime)
	if err != nil {
		t.Fatalf("生成份额失败: %v", err)
	}

	// 打印所有生成的份额，包括固定份额
	logger.Infof("生成的所有份额:")
	for i, share := range shares {
		logger.Infof("份额 #%d: (x=%s, y=%s)\n", i+1, share[0].Text(10), share[1].Text(10))
	}

	// 随机选择k个份额来恢复秘密，确保包括固定份额
	// selectedShares := append(shares[:1], shares[2:k+1]...)
	selectedShares := append(shares[:1], shares[1:k]...) // 此处简化选择过程，直接选取前k个，包含固定份额

	// 打印用于恢复的份额
	logger.Infof("用于恢复的份额:")
	for i, share := range selectedShares {
		logger.Infof("份额 #%d: (x=%s, y=%s)\n", i+1, share[0].Text(10), share[1].Text(10))
	}

	// 使用RecoverSecret恢复秘密
	recoveredSecretBytes, err := RecoverSecret(selectedShares, prime)
	if err != nil {
		t.Fatalf("恢复秘密失败: %v", err)
	}

	// 比较原始秘密和恢复后的秘密是否一致
	if !reflect.DeepEqual(secret, recoveredSecretBytes) {
		t.Errorf("原始秘密与恢复后的秘密不匹配\n原始: %s\n恢复: %s", secret, recoveredSecretBytes)
	} else {
		logger.Infof("\n成功恢复秘密\n原始: %s\n恢复: %s", secret, recoveredSecretBytes)
	}
}

// CalculateFileHash 计算并返回文件的SHA-256哈希值
func CalculateFileHash(filePath string) (*big.Int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}

	// 将哈希值转换为*big.Int
	hash := hasher.Sum(nil)
	hashInt := new(big.Int).SetBytes(hash)

	return hashInt, nil
}
