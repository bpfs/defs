package script

// ScriptFlags 是一个位掩码，定义执行脚本对时将完成的附加操作或测试
type ScriptFlags uint32

const (
	// MaxScriptSize 是原始脚本允许的最大长度
	MaxScriptSize = 10000
)

// Engine 是一个通用脚本执行引擎的结构体
type Engine struct {
	cryptoAlgo CryptoAlgorithm // 允许使用不同的加密算法
	dstack     stack           // 主要数据堆栈
}

// CryptoAlgorithm 定义了加密算法的接口
type CryptoAlgorithm interface {
	Hash(data []byte) []byte
	VerifySignature(signature, message, publicKey []byte) bool
}

// ExecutionEnvironment 定义了脚本执行环境的接口，允许脚本与外部系统交互
type ExecutionEnvironment interface {
	GetExternalData(key string) ([]byte, error)
	CallExternalAPI(api string, params ...interface{}) ([]byte, error)
}

// ResourceLimiter 用于限制脚本执行的资源使用
type ResourceLimiter struct {
	MaxExecutionTime int64 // 最大执行时间（单位：纳秒）
	MaxMemoryUsage   int64 // 最大内存使用量（单位：字节）
}
