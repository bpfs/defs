// 实现了一个签名缓存，用于提高交易验证的效率。

package script

import (
	"bytes"
	"sync"
)

// HashSize 定义了哈希值的大小，通常是双重 SHA256 哈希的大小。
const HashSize = 32

// Hash 用于多个消息和常见结构中，通常代表数据的双重 sha256 哈希。
type Hash [HashSize]byte

// sigCacheEntry 表示 SigCache 中的一个条目。SigCache 中的条目根据签名的 sigHash 进行键控。
// 在缓存命中的场景下（根据 sigHash），将额外执行签名和公钥的比较，以确保完全匹配。
// 如果两个 sigHash 发生冲突，新的 sigHash 将简单地覆盖现有条目。
type sigCacheEntry struct {
	sig    []byte // 签名数据
	pubKey []byte // 公钥数据
}

// SigCache 实现了一个结合了Schnorr和ECDSA签名验证的缓存，采用随机条目驱逐策略。
// 只有有效的签名才会被添加到缓存中。SigCache的好处有两方面：
// 首先，使用SigCache可以缓解一种DoS攻击，攻击会导致受害者的客户端由于处理攻击者
// 构造的无效交易时触发的最坏情况行为而挂起。关于被缓解的DoS攻击的详细描述可以在此处找到：
// https://bitslogger.wordpress.com/2013/01/23/fixed-bitcoin-vulnerability-explanation-why-the-signature-cache-is-a-dos-protection/。
// 其次，使用SigCache引入了签名验证优化，如果交易已经在mempool中被看到并验证过，
// 则可以加速区块中交易的验证。
type SigCache struct {
	sync.RWMutex                        // 读写锁，用于保证并发访问安全
	validSigs    map[Hash]sigCacheEntry // 有效签名的存储，映射哈希到签名缓存条目
	maxEntries   uint                   // 缓存中允许的最大条目数
}

// NewSigCache 创建并初始化一个新的 SigCache 实例。
// 参数 'maxEntries' 表示在任何特定时刻，SigCache中允许存在的最大条目数。
// 当新条目会导致缓存中的条目数超过最大值时，将随机逐出条目以腾出空间。
func NewSigCache(maxEntries uint) *SigCache {
	return &SigCache{
		validSigs:  make(map[Hash]sigCacheEntry, maxEntries),
		maxEntries: maxEntries,
	}
}

// Exists 检查是否存在一个针对公钥 'pubKey' 的签名 'sig' 在 SigCache 中的条目。
// 如果找到，则返回 true；否则返回 false。
//
// 注意：这个函数是并发安全的。读取操作不会被阻塞，除非有写入者正在添加条目到 SigCache。
func (s *SigCache) Exists(sigHash Hash, sig []byte, pubKey []byte) bool {
	s.RLock()
	entry, ok := s.validSigs[sigHash]
	s.RUnlock()

	return ok && bytes.Equal(entry.pubKey, pubKey) && bytes.Equal(entry.sig, sig)
}

// Add 向签名缓存中添加一个签名 'sig' 在 'sigHash' 下的条目，该签名使用公钥 'pubKey'。
// 如果 SigCache 已满，则随机选择一个现有条目进行逐出，以便为新条目腾出空间。
//
// 注意：这个函数是并发安全的。写入者将阻塞同时的读取者，直到函数执行完成。
func (s *SigCache) Add(sigHash Hash, sig []byte, pubKey []byte) {
	s.Lock()
	defer s.Unlock()

	if s.maxEntries <= 0 {
		return
	}

	// 如果添加这个新条目会导致超过允许的最大条目数，则逐出一个条目。
	if uint(len(s.validSigs)+1) > s.maxEntries {
		// 随机从映射中移除一个条目。依赖于 Go 映射迭代的随机起始点。
		// 需要注意的是，随机迭代起始点并不是 100% 被规范保证，但大多数 Go 编译器都支持它。
		// 最终，迭代顺序在这里并不重要，因为为了操纵哪些条目被逐出，攻击者需要能够
		// 对哈希函数执行原像攻击，以便从特定条目开始逐出。
		for sigEntry := range s.validSigs {
			delete(s.validSigs, sigEntry)
			break
		}
	}
	s.validSigs[sigHash] = sigCacheEntry{sig, pubKey}
}
