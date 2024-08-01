package script

// ScriptFlags 是一个位掩码，定义执行脚本对时将完成的附加操作或测试。
type ScriptFlags uint32

const (
	// MaxScriptSize 是原始脚本允许的最大长度。
	MaxScriptSize = 10000
)

// Engine 是一个通用脚本执行引擎的结构体。
type Engine struct {
	// 基础配置
	// flags         ScriptFlags // 用于修改引擎执行行为的附加标志。
	// scriptVersion int         // 脚本版本，用于支持不同版本的脚本语言。

	// 加密算法接口
	cryptoAlgo CryptoAlgorithm // 加密算法接口，允许使用不同的加密算法。

	// 脚本执行相关
	// scripts   [][]byte // 存储由引擎执行的脚本序列。
	// scriptIdx int      // 当前执行的脚本在脚本序列中的索引。
	// opcodeIdx int      // 当前操作码在脚本中的索引。

	// 脚本状态追踪
	// tokenizer      ScriptTokenizer // 提供脚本的令牌流，用于脚本解析。
	dstack stack // 主要数据堆栈
	// astack stack // 备用数据堆栈
	// condStack      []int           // 条件执行状态的堆栈。
	// numOps         int             // 非推送操作的计数器。

	// 脚本执行环境
	// env ExecutionEnvironment // 执行环境，提供API访问和外部交互。

	// 安全与性能
	// sigCache        *SigCache       // 缓存签名验证结果以提高性能。
	// resourceLimiter ResourceLimiter // 资源限制器，用于控制执行时间和资源使用。

	// 特定功能支持
	// taprootCtx *TaprootContext // Taproot上下文，用于支持Taproot功能。
	// witnessCtx *WitnessContext // Witness上下文，用于支持隔离见证。

	// 调试与测试
	// debugger      *Debugger      // 脚本调试器。
	// testFramework *TestFramework // 测试框架。
}

// hasFlag 返回脚本引擎实例是否设置了传递的标志。
// func (vm *Engine) hasFlag(flag ScriptFlags) bool {
// 	return vm.flags&flag == flag
// }

// CryptoAlgorithm 定义了加密算法的接口。
type CryptoAlgorithm interface {
	Hash(data []byte) []byte
	VerifySignature(signature, message, publicKey []byte) bool
}

// ExecutionEnvironment 定义了脚本执行环境的接口，允许脚本与外部系统交互。
type ExecutionEnvironment interface {
	GetExternalData(key string) ([]byte, error)
	CallExternalAPI(api string, params ...interface{}) ([]byte, error)
}

// ResourceLimiter 用于限制脚本执行的资源使用。
type ResourceLimiter struct {
	MaxExecutionTime int64
	MaxMemoryUsage   int64
}

// TaprootContext 和 WitnessContext 提供了对应功能的上下文信息。
type TaprootContext struct {
	// Taproot相关的上下文信息
}

type WitnessContext struct {
	// 隔离见证相关的上下文信息
}

// Debugger 和 TestFramework 提供调试和测试的功能。
type Debugger struct {
	// 调试功能
}

type TestFramework struct {
	// 测试框架功能
}
