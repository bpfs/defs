package uploads

// 缓冲区大小常量
const (
	// 基本IO操作的缓冲区大小
	defaultBufferSize = 32 * 1024 // 32KB

	// 数据处理的块大小
	processChunkSize = 64 * 1024 // 64KB

	// 大块操作的缓冲区大小
	largeBufferSize = 1 * 1024 * 1024 // 1MB
)
