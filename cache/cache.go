package cache

import (
	"context"
	"fmt"

	"github.com/bpfs/defs/ristretto"
	"go.uber.org/fx"
)

type NewRistrettoCacheOutput struct {
	fx.Out
	Cache *ristretto.Cache // 缓存实例
}

// NewRistrettoCache 新的缓存实例
func NewRistrettoCache(lc fx.Lifecycle) (out NewRistrettoCacheOutput, err error) {
	// 初始化 ristretto 缓存
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return out, fmt.Errorf("注册缓存实例失败: %v", err)
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})

	out.Cache = cache
	return out, nil
}

// ReadAndRemoveSliceFromCache 从缓存中读取并删除切片内容
// func ReadAndRemoveSliceFromCache(cache *ristretto.Cache, sliceHash string, isDelete bool) ([]byte, bool) {
// 	value, found := cache.Get(sliceHash)
// 	if found {
// 		if isDelete {
// 			cache.Del(sliceHash) // 删除缓存项
// 		}
// 		return value.([]byte), true
// 	}
// 	return nil, false
// }
