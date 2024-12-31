// 提供了 txscript 包使用示例的测试代码。

package script

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	"github.com/bpfs/defs/sign/rsa"
	"github.com/bpfs/defs/utils/logger"
	"golang.org/x/crypto/ripemd160"
)

// 全局版本字节和曲线
var (
	curve = elliptic.P256() // 全局椭圆曲线
)

// PublicKeyBytesToPublicKeyHash 通过公钥字节生成公钥哈希
// 参数:
//   - pubKeyBytes ([]byte): 公钥字节
//
// 返回值:
//   - []byte: 公钥哈希
//   - bool: 是否成功
func PublicKeyBytesToPublicKeyHash(pubKeyBytes []byte) ([]byte, bool) {
	if !IsValidPublicKey(pubKeyBytes) {
		return nil, false
	}
	return HashPublicKey(pubKeyBytes), true
}

// IsValidPublicKey 检查公钥字节是否有效
// 参数:
//   - pubKeyBytes ([]byte): 公钥字节
//
// 返回值:
//   - bool: 公钥是否有效的布尔值
func IsValidPublicKey(pubKeyBytes []byte) bool {
	_, err := UnmarshalPublicKey(pubKeyBytes)
	return err == nil
}

// UnmarshalPublicKey 将字节序列反序列化为ECDSA公钥
// 参数:
//   - pubKeyBytes ([]byte): 公钥的字节表示
//
// 返回值:
//   - ecdsa.PublicKey: 反序列化后的ECDSA公钥
//   - error: 失败时的错误信息
func UnmarshalPublicKey(pubKeyBytes []byte) (ecdsa.PublicKey, error) {
	// 使用全局曲线
	x, y := elliptic.Unmarshal(curve, pubKeyBytes)
	if x == nil || y == nil {
		err := errors.New("无效的公钥字节")
		return ecdsa.PublicKey{}, err
	}
	// 返回反序列化后的公钥
	return ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// HashPublicKey 将公钥字节进行SHA-256和RIPEMD-160双重哈希
// 参数:
//   - pubKeyBytes ([]byte): 公钥的字节表示
//
// 返回值:
//   - []byte: 公钥的哈希值
func HashPublicKey(pubKeyBytes []byte) []byte {
	// 对公钥字节进行SHA-256哈希
	shaHash := sha256.Sum256(pubKeyBytes)
	// 创建RIPEMD-160哈希器
	ripeHasher := ripemd160.New()
	// 写入SHA-256哈希结果到RIPEMD-160哈希器
	ripeHasher.Write(shaHash[:])
	// 返回RIPEMD-160哈希值
	return ripeHasher.Sum(nil)
}

// 此示例演示了创建一个向比特币地址付款的脚本。
// 它还打印创建的脚本十六进制并使用 DisasmString 函数显示反汇编的脚本。
// P2PKH（支付给公钥哈希值）
// func TestExamplePayToAddrScript(t *testing.T) {
// // 将发送硬币的地址解析为btcutil.Address，这对于确保地址的准确性和确定地址类型很有用。
// // 即将到来的 PayToAddrScript 调用也需要它。
// addressStr := "12gpXQVcCL2qhTNQgyLVdCFG2Qs2px98nV"
// // DecodeAddress 对地址的字符串编码进行解码，如果 addr 是已知地址类型的有效编码，则返回该地址。
// // address, err := btcutil.DecodeAddress(addressStr, &chaincfg.MainNetParams)
// // if err != nil {
// // 	logger.Println(err)
// // 	return
// // }
// // logger.Printf("address:\t%v\n", address)

// // ScriptAddress 返回将地址插入 txout 脚本时要使用的地址的原始字节。
// // pubKeyHash := address.ScriptAddress()
// pubKeyHash, err := wallet.GetPubKeyHash(addressStr)
// if err != nil {
// 	logger.Println("Error:", err)
// 	return
// }
// logger.Println("Public Key Hash:", hex.EncodeToString(pubKeyHash))
// script, err := NewScriptBuilder().
// 	AddOp(OP_DUP).AddOp(OP_HASH160).
// 	AddData(pubKeyHash).
// 	AddOp(OP_EQUALVERIFY).AddOp(OP_CHECKSIG).
// 	Script()
// if err != nil {
// 	logger.Println(err)
// 	return
// }

// logger.Printf("十六进制脚本:\t%x\n", script)

// // 返回传递的脚本是否是标准的 支付到公钥哈希脚本。
// logger.Printf("%v\n", IsPayToPubKeyHash(script))

// // 将反汇编脚本格式化为一行打印
// disasm, err := DisasmString(script)
// if err != nil {
// 	logger.Println(err)
// 	return
// }
// logger.Printf("脚本反汇编:\t%s\n", disasm)

// // 验证脚本中的公钥哈希
// if VerifyScriptPubKeyHash(script, pubKeyHash) {
// 	logger.Println("脚本验证成功，公钥哈希匹配")
// } else {
// 	logger.Println("脚本验证失败，公钥哈希不匹配")
// }

// 输出:
// Public Key Hash: 128004ff2fcaf13b2b91eb654b1dc2b674f7ec61
// 十六进制脚本: 76a914128004ff2fcaf13b2b91eb654b1dc2b674f7ec6188ac
// 脚本反汇编: OP_DUP OP_HASH160 128004ff2fcaf13b2b91eb654b1dc2b674f7ec61 OP_EQUALVERIFY OP_CHECKSIG
// }

func TestPayToPubKeyScriptECDSA(t *testing.T) {
	// 生成一个新的ECDSA私钥
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("无法生成私钥: %v", err)
	}

	// 获取公钥的字节表示
	pubKeyBytes := elliptic.Marshal(privateKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y)
	logger.Printf("获取的公钥字节:\t%s\n", hex.EncodeToString(pubKeyBytes))

	// 构建P2PK脚本
	script, err := NewScriptBuilder().
		// AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyBytes). // 直接添加公钥
		// AddOp(OP_EQUALVERIFY).AddOp(OP_CHECKSIG). // 添加检查签名操作
		AddOp(OP_CHECKSIG). // 添加检查签名操作
		Script()
	if err != nil {
		logger.Println("Error building script:", err)
		return
	}
	logger.Printf("十六进制脚本:\t%x\n", script)

	// 调用 DisassembleScript 来反汇编脚本
	disassembledScript := DisassembleScript(script)
	logger.Printf("反汇编脚本:\t%s\n", disassembledScript)

	// 打印脚本
	// logger.Println("Script:\t\t", hex.EncodeToString(script))

	// 从脚本中提取公钥
	pubKey, err := ExtractPubKeyFromP2PKScriptToECDSA(script)
	if err != nil {
		logger.Println("提取公钥时出错:", err)
		return
	}

	// 打印提取的公钥
	logger.Println("公钥 X 坐标:", pubKey.X.Text(16))
	logger.Println("公钥 Y 坐标:", pubKey.Y.Text(16))

	pubKeyBytes2 := elliptic.Marshal(pubKey.Curve, pubKey.X, pubKey.Y)
	logger.Printf("获取的公钥字节:\t%s\n", hex.EncodeToString(pubKeyBytes2))
}
func TestPayToPubKeyHashScriptECDSA(t *testing.T) {
	// 生成一个新的ECDSA私钥
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("无法生成私钥: %v", err)
	}

	// 获取公钥的字节表示
	pubKeyBytes := elliptic.Marshal(privateKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y)
	logger.Printf("获取的公钥字节:\t%s\n", hex.EncodeToString(pubKeyBytes))

	// 通过公钥字节生成公钥哈希
	pubKeyHash, ok := PublicKeyBytesToPublicKeyHash(pubKeyBytes)
	if !ok {
		t.Fatalf("通过公钥字节生成公钥哈希: %v", err)
	}
	logger.Printf("公钥哈希:\t%x\n", pubKeyHash)

	script, err := NewScriptBuilder().
		AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyHash). // 直接添加公钥哈希
		AddOp(OP_EQUALVERIFY).AddOp(OP_CHECKSIG).
		Script()
	if err != nil {
		logger.Println("Error building script:", err)
		return
	}
	logger.Printf("十六进制脚本:\t%x\n", script)

	// 调用 DisassembleScript 来反汇编脚本
	disassembledScript := DisassembleScript(script)
	logger.Printf("反汇编脚本:\t%s\n", disassembledScript)

	// 从P2PKH脚本中提取公钥哈希
	extractedPubKeyHash, err := ExtractPubKeyHashFromScript(script)
	if err != nil {
		t.Fatalf("无法从脚本中提取公钥哈希: %v", err)
	}
	logger.Printf("从脚本中提取的公钥哈希:\t%x\n", extractedPubKeyHash)
}

func TestPayToPubKeyHashScriptRSA(t *testing.T) {
	// 生成密钥对
	seedData := []byte("your_seed_data_here")
	_, publicKey, err := rsa.GenerateKeysFromSeed(seedData, 2048)
	if err != nil {
		t.Error("Error generating keys:", err)
		return
	}

	publicKeyBytes, err := rsa.PublicKeyToBytes(publicKey)
	if err != nil {
		t.Error("Error publicKeyBytes:", err)
		return
	}
	logger.Printf("获取的公钥字节:\t%s\n", hex.EncodeToString(publicKeyBytes))

	// 构建P2PK脚本
	script, err := NewScriptBuilder().
		AddData(publicKeyBytes). // 直接添加公钥
		AddOp(OP_CHECKSIG).      // 添加检查签名操作
		Script()
	if err != nil {
		t.Error("Error script:", err)
		return
	}
	logger.Printf("十六进制脚本:\t%x\n", script)

	// 调用 DisassembleScript 来反汇编脚本
	disassembledScript := DisassembleScript(script)
	logger.Printf("反汇编脚本:\t%s\n", disassembledScript)

	// 从脚本中提取公钥
	pubKey, err := ExtractPubKeyFromP2PKScriptToRSA(script)
	if err != nil {
		logger.Println("提取公钥时出错:", err)
		return
	}

	pubKeyBytes, err := rsa.PublicKeyToBytes(pubKey)
	if err != nil {
		t.Error("Error publicKeyBytes:", err)
		return
	}
	logger.Printf("提取的公钥字节:\t%s\n", hex.EncodeToString(pubKeyBytes))
}

// ExtractPubKeyFromP2PKScript 从P2PK脚本中提取公钥
// func ExtractPubKeyFromP2PKScript(p2pkScript []byte) (*ecdsa.PublicKey, error) {
// 	// 检查脚本是否以OP_CHECKSIG结尾
// 	if len(p2pkScript) < 2 || p2pkScript[len(p2pkScript)-1] != 0xAC { // OP_CHECKSIG的十六进制代码是0xAC
// 		return nil, fmt.Errorf("无效的P2PK脚本")
// 	}

// 	// 获取公钥字节，假设脚本以OP_DUP (0x76), OP_HASH160 (0xA9), 公钥长度, 公钥, OP_EQUALVERIFY (0x88), OP_CHECKSIG (0xAC)的顺序排列
// 	if p2pkScript[0] != 0x76 || p2pkScript[1] != 0xA9 {
// 		return nil, fmt.Errorf("脚本不符合预期的P2PK格式")
// 	}

// 	// OP_HASH160后面是公钥长度，然后是公钥本身
// 	pubKeyLength := int(p2pkScript[2])
// 	if len(p2pkScript) != 3+pubKeyLength+2 {
// 		return nil, fmt.Errorf("脚本长度不符合预期")
// 	}
// 	pubKeyBytes := p2pkScript[3 : 3+pubKeyLength]

// 	logger.Printf("提取的公钥字节: %x\n", pubKeyBytes) // 打印公钥字节

// 	x, y := elliptic.Unmarshal(elliptic.P256(), pubKeyBytes)
// 	if x == nil || y == nil {
// 		return nil, fmt.Errorf("无法解析公钥")
// 	}

// 	return &ecdsa.PublicKey{
// 		Curve: elliptic.P256(),
// 		X:     x,
// 		Y:     y,
// 	}, nil
// }

// 测试压缩和解压ECDSA公钥的功能
func TestCompressAndDecompressPubKey(t *testing.T) {
	// 生成一个新的ECDSA私钥
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("无法生成私钥: %v", err)
	}

	// 获取公钥的字节表示
	pubKeyBytes := elliptic.Marshal(privateKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y)
	logger.Printf("获取的公钥字节:\t%s\n", hex.EncodeToString(pubKeyBytes))

	originalPubKey := &privateKey.PublicKey

	// 压缩公钥
	compressedPubKey := CompressPubKey(originalPubKey)
	logger.Printf("公钥的公钥字节:\t%s\n", hex.EncodeToString(compressedPubKey))

	// 解压公钥
	decompressedPubKey, err := DecompressPubKey(elliptic.P256(), compressedPubKey)
	if err != nil {
		t.Fatalf("解压公钥失败: %v", err)
	}

	// 解压公钥的字节表示
	decompressedPubKeyBytes := elliptic.Marshal(privateKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y)
	logger.Printf("获取的公钥字节:\t%s\n", hex.EncodeToString(decompressedPubKeyBytes))

	// 比较原始公钥和解压后的公钥
	if !reflect.DeepEqual(originalPubKey, decompressedPubKey) {
		t.Errorf("原始公钥和解压后的公钥不匹配")
	}
}
